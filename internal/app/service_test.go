package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/domain"
)

// fixedClock implements Clock returning a fixed instant.
type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

// mockStore implements SecretStore for tests.
type mockStore struct {
	saveErr error

	claimMeta Meta
	claimData string
	claimSize int64
	claimErr  error

	ackErr error

	// captured on Save
	savedID      string
	savedMeta    Meta
	savedSize    int64
	savedExpires time.Time
	saveCalled   bool

	claimCalled       bool
	claimID           string
	claimHash         string
	claimRetry        bool
	claimClaimedUntil time.Time

	ackCalled bool
	ackID     string
	ackHash   string
}

func (m *mockStore) Save(ctx context.Context, id string, meta Meta, r io.Reader, size int64, expiresAt time.Time) error {
	_ = ctx
	_ = r
	m.saveCalled = true
	m.savedID = id
	m.savedMeta = meta
	m.savedSize = size
	m.savedExpires = expiresAt
	return m.saveErr
}

func (m *mockStore) Claim(ctx context.Context, id, claimHash string, retry bool, claimedUntil time.Time) (Claimed, error) {
	_ = ctx
	m.claimCalled = true
	m.claimID = id
	m.claimHash = claimHash
	m.claimRetry = retry
	m.claimClaimedUntil = claimedUntil
	if m.claimErr != nil {
		return Claimed{}, m.claimErr
	}
	return Claimed{
		Meta:         m.claimMeta,
		Body:         io.NopCloser(strings.NewReader(m.claimData)),
		Size:         m.claimSize,
		ClaimedUntil: claimedUntil,
	}, nil
}

func (m *mockStore) Ack(ctx context.Context, id, claimHash string) error {
	_ = ctx
	m.ackCalled = true
	m.ackID = id
	m.ackHash = claimHash
	return m.ackErr
}

func (m *mockStore) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	_ = ctx
	_ = t
	return 0, nil
}
func (m *mockStore) Reconcile(ctx context.Context) error { _ = ctx; return nil }

// metricsRecorder captures service metric increments for assertions.
type metricsRecorder struct {
	counts map[string]int64
}

// Inc records the named metric delta.
func (m *metricsRecorder) Inc(name string, delta int64) {
	if m.counts == nil {
		m.counts = make(map[string]int64)
	}
	m.counts[name] += delta
}

// count returns the recorded total for name.
func (m *metricsRecorder) count(name string) int64 {
	if m.counts == nil {
		return 0
	}
	return m.counts[name]
}

// mustClaimToken returns a valid claim token for tests.
func mustClaimToken(t *testing.T) domain.ClaimToken {
	t.Helper()
	tok, err := domain.NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	return tok
}

func TestServiceCreateSecretSuccess(t *testing.T) {
	ms := &mockStore{}
	now := time.Unix(1700000000, 0)
	svc := &Service{Store: ms, Clock: fixedClock{now: now}, MaxBytes: 1024, MinTTL: time.Minute, MaxTTL: 10 * time.Minute}
	data := "ciphertext"
	ttl := 2 * time.Minute
	id, exp, err := svc.CreateSecret(context.Background(), strings.NewReader(data), int64(len(data)), 1, "nonce123", ttl)
	if err != nil {
		t.Fatalf("CreateSecret error: %v", err)
	}
	if !id.Valid() {
		t.Fatalf("returned id invalid: %s", id)
	}
	if exp != now.Add(ttl) {
		t.Fatalf("expiry mismatch: got %v want %v", exp, now.Add(ttl))
	}
	if !ms.saveCalled {
		t.Fatalf("expected Save to be called")
	}
	if ms.savedID != id.String() {
		t.Fatalf("savedID mismatch")
	}
	if ms.savedMeta.Version != 1 || ms.savedMeta.NonceB64u != "nonce123" {
		t.Fatalf("meta mismatch: %+v", ms.savedMeta)
	}
	if ms.savedSize != int64(len(data)) {
		t.Fatalf("size mismatch: %d", ms.savedSize)
	}
	if ms.savedExpires != exp {
		t.Fatalf("expires mismatch: %v vs %v", ms.savedExpires, exp)
	}
}

func TestServiceCreateSecretTTLInvalid(t *testing.T) {
	ms := &mockStore{}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 1024, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	// below min
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader("a"), 1, 1, "n", 30*time.Second); err != domain.ErrTTLInvalid {
		t.Fatalf("expected ErrTTLInvalid for below min, got %v", err)
	}
	// above max
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader("a"), 1, 1, "n", 10*time.Minute); err != domain.ErrTTLInvalid {
		t.Fatalf("expected ErrTTLInvalid for above max, got %v", err)
	}
}

func TestServiceCreateSecretSizeValidation(t *testing.T) {
	ms := &mockStore{}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 10, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader(""), 0, 1, "n", time.Minute); err != ErrSizeExceeded {
		t.Fatalf("expected ErrSizeExceeded for size 0, got %v", err)
	}
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader("01234567890"), 11, 1, "n", time.Minute); err != ErrSizeExceeded {
		t.Fatalf("expected ErrSizeExceeded for oversize, got %v", err)
	}
}

func TestServiceCreateSecretStoreError(t *testing.T) {
	boom := errors.New("boom")
	ms := &mockStore{saveErr: boom}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 100, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	_, _, err := svc.CreateSecret(context.Background(), strings.NewReader("abc"), 3, 1, "n", 2*time.Minute)
	if err != boom {
		t.Fatalf("expected store error propagation, got %v", err)
	}
	if !ms.saveCalled {
		t.Fatalf("expected save called")
	}
}

func TestServiceClaimInvalidID(t *testing.T) {
	ms := &mockStore{}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 100, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	if _, err := svc.Claim(context.Background(), "not-an-id", ""); err != domain.ErrInvalidID {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}
	if ms.claimCalled {
		t.Fatalf("store should not be called on invalid id")
	}
}

func TestServiceClaim(t *testing.T) {
	now := time.Unix(1700000500, 0).UTC()
	validID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	retryToken := mustClaimToken(t)
	storeErr := errors.New("claim failed")
	data := "ciphertext"

	tests := []struct {
		name       string
		token      string
		lease      time.Duration
		storeErr   error
		wantErr    error
		wantRetry  bool
		wantLease  time.Duration
		wantCalled bool
	}{
		{name: "fresh claim issues token with default lease", wantLease: DefaultClaimLease, wantCalled: true},
		{name: "retry passes parsed token hash", token: retryToken.String(), wantRetry: true, wantLease: DefaultClaimLease, wantCalled: true},
		{name: "custom lease", lease: 90 * time.Second, wantLease: 90 * time.Second, wantCalled: true},
		{name: "invalid token", token: "bad-token", wantErr: domain.ErrInvalidClaim},
		{name: "store error", storeErr: storeErr, wantErr: storeErr, wantLease: DefaultClaimLease, wantCalled: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &mockStore{
				claimMeta: Meta{Version: 2, NonceB64u: "nonceX"},
				claimData: data,
				claimSize: int64(len(data)),
				claimErr:  tc.storeErr,
			}
			svc := &Service{
				Store:      ms,
				Clock:      fixedClock{now: now},
				MaxBytes:   100,
				MinTTL:     time.Minute,
				MaxTTL:     5 * time.Minute,
				ClaimLease: tc.lease,
			}

			res, err := svc.Claim(context.Background(), validID, tc.token)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Claim error = %v, want %v", err, tc.wantErr)
			}
			if ms.claimCalled != tc.wantCalled {
				t.Fatalf("claimCalled = %v, want %v", ms.claimCalled, tc.wantCalled)
			}
			if tc.wantErr != nil {
				return
			}
			if res.Meta.Version != 2 || res.Meta.NonceB64u != "nonceX" {
				t.Fatalf("meta mismatch: %+v", res.Meta)
			}
			b, readErr := io.ReadAll(res.Body)
			if readErr != nil {
				t.Fatalf("ReadAll: %v", readErr)
			}
			if string(b) != data {
				t.Fatalf("data mismatch: %s", string(b))
			}
			if res.Size != int64(len(data)) {
				t.Fatalf("size mismatch: %d", res.Size)
			}
			if ms.claimID != validID {
				t.Fatalf("claim id = %q, want %q", ms.claimID, validID)
			}
			if ms.claimRetry != tc.wantRetry {
				t.Fatalf("retry = %v, want %v", ms.claimRetry, tc.wantRetry)
			}
			wantToken := res.Token
			if tc.token != "" {
				wantToken = retryToken
				if res.Token.String() != tc.token {
					t.Fatalf("retry token = %q, want %q", res.Token.String(), tc.token)
				}
			} else if _, parseErr := domain.ParseClaimToken(res.Token.String()); parseErr != nil {
				t.Fatalf("fresh token is invalid: %v", parseErr)
			}
			if ms.claimHash != wantToken.Hash() {
				t.Fatalf("claim hash = %q, want %q", ms.claimHash, wantToken.Hash())
			}
			wantUntil := now.Add(tc.wantLease)
			if !ms.claimClaimedUntil.Equal(wantUntil) {
				t.Fatalf("store claimedUntil = %v, want %v", ms.claimClaimedUntil, wantUntil)
			}
			if !res.ClaimedUntil.Equal(wantUntil) {
				t.Fatalf("result claimedUntil = %v, want %v", res.ClaimedUntil, wantUntil)
			}
		})
	}
}

func TestServiceAck(t *testing.T) {
	validID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	validToken := mustClaimToken(t)
	storeErr := errors.New("ack failed")

	tests := []struct {
		name       string
		id         string
		token      string
		storeErr   error
		wantErr    error
		wantCalled bool
		wantMetric int64
	}{
		{name: "success increments consumed metric", id: validID, token: validToken.String(), wantCalled: true, wantMetric: 1},
		{name: "store error propagates without metric", id: validID, token: validToken.String(), storeErr: storeErr, wantErr: storeErr, wantCalled: true},
		{name: "invalid id", id: "bad-id", token: validToken.String(), wantErr: domain.ErrInvalidID},
		{name: "invalid token", id: validID, token: "bad-token", wantErr: domain.ErrInvalidClaim},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &mockStore{ackErr: tc.storeErr}
			metrics := &metricsRecorder{}
			svc := &Service{
				Store:    ms,
				Clock:    fixedClock{now: time.Now()},
				MaxBytes: 100,
				MinTTL:   time.Minute,
				MaxTTL:   5 * time.Minute,
				Metrics:  metrics,
			}

			err := svc.Ack(context.Background(), tc.id, tc.token)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Ack error = %v, want %v", err, tc.wantErr)
			}
			if ms.ackCalled != tc.wantCalled {
				t.Fatalf("ackCalled = %v, want %v", ms.ackCalled, tc.wantCalled)
			}
			if tc.wantCalled {
				if ms.ackID != tc.id {
					t.Fatalf("ack id = %q, want %q", ms.ackID, tc.id)
				}
				if ms.ackHash != validToken.Hash() {
					t.Fatalf("ack hash = %q, want %q", ms.ackHash, validToken.Hash())
				}
			}
			if got := metrics.count("secrets_consumed_total"); got != tc.wantMetric {
				t.Fatalf("consumed metric = %d, want %d", got, tc.wantMetric)
			}
		})
	}
}
