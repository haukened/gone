package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// fakeRequests implements RequestStore and records what the service passes.
type fakeRequests struct {
	err        error
	expires    time.Time
	status     RequestStatus
	claimed    Claimed
	fillHash   string
	manageHash string
	claimHash  string
	retry      bool
	ttl        time.Duration
	meta       Meta
	body       string
}

func (f *fakeRequests) CreateRequest(_ context.Context, _, fillHash, manageHash string, ttl time.Duration, expiresAt time.Time) error {
	f.fillHash, f.manageHash, f.ttl, f.expires = fillHash, manageHash, ttl, expiresAt
	return f.err
}

func (f *fakeRequests) RequestOpen(_ context.Context, _, fillHash string) (time.Time, error) {
	f.fillHash = fillHash
	return f.expires, f.err
}

func (f *fakeRequests) Fill(_ context.Context, _, fillHash string, meta Meta, r io.Reader, _ int64) (time.Time, error) {
	b, _ := io.ReadAll(r)
	f.fillHash, f.meta, f.body = fillHash, meta, string(b)
	return f.expires, f.err
}

func (f *fakeRequests) RequestStatus(_ context.Context, _, manageHash string) (RequestStatus, error) {
	f.manageHash = manageHash
	return f.status, f.err
}

func (f *fakeRequests) ClaimReply(_ context.Context, _, manageHash, claimHash string, retry bool, _ time.Time) (Claimed, error) {
	f.manageHash, f.claimHash, f.retry = manageHash, claimHash, retry
	return f.claimed, f.err
}

func (f *fakeRequests) CancelRequest(_ context.Context, _, manageHash string) error {
	f.manageHash = manageHash
	return f.err
}

type reqMetrics map[string]int64

func (m reqMetrics) Inc(name string, d int64) { m[name] += d }

const reqTestID = "0123456789abcdef0123456789abcdef"

func reqService(f *fakeRequests) (*Service, reqMetrics) {
	m := reqMetrics{}
	return &Service{
		Store: &mockStore{}, Requests: f, Clock: fixedClock{now: time.Unix(1700000000, 0)},
		MaxBytes: 100, MinTTL: time.Minute, MaxTTL: time.Hour, Metrics: m,
	}, m
}

func TestCreateRequest(t *testing.T) {
	f := &fakeRequests{}
	s, m := reqService(f)
	got, err := s.CreateRequest(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if got.FillToken.Hash() != f.fillHash || got.ManageToken.Hash() != f.manageHash || f.ttl != 30*time.Minute {
		t.Fatalf("stored hashes/ttl mismatch: %+v %+v", got, f)
	}
	if !got.ExpiresAt.Equal(time.Unix(1700000000, 0).Add(30*time.Minute)) || m["requests_created_total"] != 1 {
		t.Fatalf("expires %v metrics %v", got.ExpiresAt, m)
	}
	if _, err := s.CreateRequest(context.Background(), 2*time.Hour); !errors.Is(err, domain.ErrTTLInvalid) {
		t.Fatalf("ttl err = %v", err)
	}
	f.err = errors.New("disk")
	if _, err := s.CreateRequest(context.Background(), time.Minute); err == nil {
		t.Fatalf("store error swallowed")
	}
}

func TestRequestsDisabled(t *testing.T) {
	s := &Service{Store: &mockStore{}, Clock: fixedClock{}}
	ctx := context.Background()
	if _, err := s.CreateRequest(ctx, time.Minute); !errors.Is(err, ErrRequestsDisabled) {
		t.Errorf("CreateRequest err = %v", err)
	}
	if _, err := s.RequestOpen(ctx, reqTestID, ""); !errors.Is(err, ErrRequestsDisabled) {
		t.Errorf("RequestOpen err = %v", err)
	}
	if _, err := s.RequestStatus(ctx, reqTestID, ""); !errors.Is(err, ErrRequestsDisabled) {
		t.Errorf("RequestStatus err = %v", err)
	}
}

func TestFillValidation(t *testing.T) {
	fill, _ := domain.NewFillToken()
	nonce := domain.EncodeB64URL(make([]byte, domain.NonceSize))
	f := &fakeRequests{expires: time.Unix(1700001800, 0)}
	s, m := reqService(f)
	ctx := context.Background()
	exp, err := s.Fill(ctx, reqTestID, fill.String(), strings.NewReader("ct"), 2, 3, nonce)
	if err != nil || !exp.Equal(f.expires) || f.fillHash != fill.Hash() || f.body != "ct" || f.meta.Version != 3 {
		t.Fatalf("Fill = %v, %v; fake %+v", exp, err, f)
	}
	if m["requests_filled_total"] != 1 {
		t.Fatalf("metrics = %v", m)
	}
	cases := []struct {
		name    string
		id      string
		fill    string
		size    int64
		version uint8
		nonce   string
		want    error
	}{
		{"bad id", "x", fill.String(), 2, 3, nonce, domain.ErrInvalidID},
		{"bad fill", reqTestID, "x", 2, 3, nonce, domain.ErrInvalidFill},
		{"zero size", reqTestID, fill.String(), 0, 3, nonce, ErrSizeExceeded},
		{"too large", reqTestID, fill.String(), 101, 3, nonce, ErrSizeExceeded},
		{"secret version", reqTestID, fill.String(), 2, 1, nonce, domain.ErrInvalidVersion},
		{"bad nonce", reqTestID, fill.String(), 2, 3, "x", domain.ErrInvalidNonce},
	}
	for _, c := range cases {
		if _, err := s.Fill(ctx, c.id, c.fill, strings.NewReader(""), c.size, c.version, c.nonce); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	f.err = ErrNotFound
	if _, err := s.Fill(ctx, reqTestID, fill.String(), strings.NewReader("ct"), 2, 3, nonce); !errors.Is(err, ErrNotFound) {
		t.Fatalf("store err = %v", err)
	}
}

func TestRequestOpenAndStatus(t *testing.T) {
	fill, _ := domain.NewFillToken()
	manage, _ := domain.NewManageToken()
	f := &fakeRequests{expires: time.Unix(5, 0), status: RequestStatus{Ready: true}}
	s, _ := reqService(f)
	ctx := context.Background()
	if exp, err := s.RequestOpen(ctx, reqTestID, fill.String()); err != nil || !exp.Equal(f.expires) || f.fillHash != fill.Hash() {
		t.Fatalf("RequestOpen = %v, %v", exp, err)
	}
	if _, err := s.RequestOpen(ctx, "bad", fill.String()); !errors.Is(err, domain.ErrInvalidID) {
		t.Fatalf("RequestOpen bad id err = %v", err)
	}
	st, err := s.RequestStatus(ctx, reqTestID, manage.String())
	if err != nil || !st.Ready || f.manageHash != manage.Hash() {
		t.Fatalf("RequestStatus = %+v, %v", st, err)
	}
	if _, err := s.RequestStatus(ctx, reqTestID, "short"); !errors.Is(err, domain.ErrInvalidManage) {
		t.Fatalf("RequestStatus bad token err = %v", err)
	}
}

func TestClaimAndAckReply(t *testing.T) {
	manage, _ := domain.NewManageToken()
	f := &fakeRequests{claimed: Claimed{Size: 3}}
	s, m := reqService(f)
	ctx := context.Background()
	res, err := s.ClaimReply(ctx, reqTestID, manage.String(), "")
	if err != nil || f.retry || f.claimHash != res.Token.Hash() || f.manageHash != manage.Hash() {
		t.Fatalf("fresh ClaimReply = %+v, %v", res, err)
	}
	if _, err := s.ClaimReply(ctx, reqTestID, manage.String(), res.Token.String()); err != nil || !f.retry {
		t.Fatalf("retry ClaimReply err = %v retry=%v", err, f.retry)
	}
	if _, err := s.ClaimReply(ctx, reqTestID, manage.String(), "bad"); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("bad claim err = %v", err)
	}
	if _, err := s.ClaimReply(ctx, reqTestID, "bad", ""); !errors.Is(err, domain.ErrInvalidManage) {
		t.Fatalf("bad manage err = %v", err)
	}
	f.err = ErrNotFound
	if _, err := s.ClaimReply(ctx, reqTestID, manage.String(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("store err = %v", err)
	}
	if err := s.AckReply(ctx, reqTestID, res.Token.String()); err != nil || m["requests_opened_total"] != 1 {
		t.Fatalf("AckReply = %v metrics %v", err, m)
	}
	if err := s.AckReply(ctx, "bad", res.Token.String()); !errors.Is(err, domain.ErrInvalidID) {
		t.Fatalf("AckReply bad id err = %v", err)
	}
	if err := s.AckReply(ctx, reqTestID, "bad"); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("AckReply bad claim err = %v", err)
	}
	s.Store = &mockStore{ackErr: ErrNotFound}
	if err := s.AckReply(ctx, reqTestID, res.Token.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AckReply store err = %v", err)
	}
}

func TestCancelRequest(t *testing.T) {
	manage, _ := domain.NewManageToken()
	f := &fakeRequests{}
	s, m := reqService(f)
	ctx := context.Background()
	if err := s.CancelRequest(ctx, reqTestID, manage.String()); err != nil || m["requests_cancelled_total"] != 1 {
		t.Fatalf("CancelRequest = %v metrics %v", err, m)
	}
	if err := s.CancelRequest(ctx, reqTestID, "bad"); !errors.Is(err, domain.ErrInvalidManage) {
		t.Fatalf("bad token err = %v", err)
	}
	f.err = ErrNotFound
	if err := s.CancelRequest(ctx, reqTestID, manage.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("store err = %v", err)
	}
}
