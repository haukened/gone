package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

var testFill = domain.FillToken(strings.Repeat("A", 43))

// recorded is what a test server saw.
type recorded struct {
	method, path   string
	fill, manage   string
	version, nonce string
	ttl, claim     string
	body           string
}

// reqServer answers every request with status and body, recording it.
func reqServer(t *testing.T, status int, body string, hdr http.Header) (*Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*rec = recorded{
			method: r.Method, path: r.URL.Path, fill: r.Header.Get(headerFill), manage: r.Header.Get(headerManage),
			version: r.Header.Get(headerVersion), nonce: r.Header.Get(headerNonce), ttl: r.Header.Get("X-Gone-TTL"),
			claim: r.Header.Get(headerClaim), body: string(b),
		}
		for k, vs := range hdr {
			w.Header()[k] = vs
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	return c, rec
}

func TestCreateRequest(t *testing.T) {
	ok := `{"id":"` + testID.String() + `","expires_at":"2030-01-01T00:00:00Z","manage_token":"` + testManage.String() + `","fill_token":"` + testFill.String() + `"}`
	c, rec := reqServer(t, http.StatusCreated, ok, nil)
	got, err := c.CreateRequest(context.Background(), 30*time.Minute)
	if err != nil || got.ID != testID || got.ManageToken != testManage || got.FillToken != testFill || got.ExpiresAt.IsZero() {
		t.Fatalf("CreateRequest = %+v, %v", got, err)
	}
	if rec.method != http.MethodPost || rec.path != "/api/request" || rec.ttl != "30m0s" || rec.body != "" {
		t.Fatalf("request = %+v", rec)
	}
	bad := []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusCreated, `{"id":"x"}`, ErrProtocol},
		{http.StatusCreated, `{"id":"` + testID.String() + `","expires_at":"2030-01-01T00:00:00Z","manage_token":"x","fill_token":"y"}`, ErrProtocol},
		{http.StatusCreated, `nope`, ErrProtocol},
		{http.StatusCreated, strings.Repeat("x", maxJSONBody+1), ErrTooLarge},
		{http.StatusBadRequest, ``, ErrRejected},
		{http.StatusTooManyRequests, ``, ErrRateLimited},
	}
	for _, b := range bad {
		c, _ := reqServer(t, b.status, b.body, nil)
		if _, err := c.CreateRequest(context.Background(), time.Minute); !errors.Is(err, b.want) {
			t.Errorf("%d %q: err = %v, want %v", b.status, b.body[:min(len(b.body), 20)], err, b.want)
		}
	}
}

func TestRequestOpenAndFill(t *testing.T) {
	c, rec := reqServer(t, http.StatusOK, `{"state":"open","expires_at":"2030-01-01T00:00:00Z"}`, nil)
	exp, err := c.RequestOpen(context.Background(), testID, testFill)
	if err != nil || exp.IsZero() || rec.path != "/api/request/"+testID.String() || rec.fill != testFill.String() || rec.method != http.MethodGet {
		t.Fatalf("RequestOpen = %v, %v; %+v", exp, err, rec)
	}
	for _, body := range []string{`{"state":"closed","expires_at":"2030-01-01T00:00:00Z"}`, `{"state":"open"}`, `x`} {
		c, _ := reqServer(t, http.StatusOK, body, nil)
		if _, err := c.RequestOpen(context.Background(), testID, testFill); !errors.Is(err, ErrProtocol) {
			t.Errorf("%s: err = %v", body, err)
		}
	}
	c, _ = reqServer(t, http.StatusNotFound, ``, nil)
	if _, err := c.RequestOpen(context.Background(), testID, testFill); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 err = %v", err)
	}

	c, rec = reqServer(t, http.StatusCreated, `{"expires_at":"2030-01-01T00:00:00Z"}`, nil)
	exp, err = c.Fill(context.Background(), testID, testFill, make([]byte, 12), []byte("blob"))
	if err != nil || exp.IsZero() {
		t.Fatalf("Fill = %v, %v", exp, err)
	}
	if rec.method != http.MethodPut || rec.path != "/api/request/"+testID.String()+"/reply" || rec.version != "3" ||
		rec.nonce != domain.EncodeB64URL(make([]byte, 12)) || rec.fill != testFill.String() || rec.body != "blob" {
		t.Fatalf("fill request = %+v", rec)
	}
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{{http.StatusCreated, `{}`, ErrProtocol}, {http.StatusNotFound, ``, ErrNotFound}, {http.StatusRequestEntityTooLarge, ``, ErrTooLarge}} {
		c, _ := reqServer(t, tc.status, tc.body, nil)
		if _, err := c.Fill(context.Background(), testID, testFill, nil, []byte("b")); !errors.Is(err, tc.want) {
			t.Errorf("fill %d: err = %v", tc.status, err)
		}
	}
}

func TestRequestStatusAndCancel(t *testing.T) {
	for state, ready := range map[string]bool{"waiting": false, "ready": true} {
		c, rec := reqServer(t, http.StatusOK, `{"state":"`+state+`","created_at":"2030-01-01T00:00:00Z","expires_at":"2030-01-02T00:00:00Z"}`, nil)
		st, err := c.RequestStatus(context.Background(), testID, testManage)
		if err != nil || st.Ready != ready || st.CreatedAt.IsZero() || rec.path != "/api/request/"+testID.String()+"/status" || rec.manage != testManage.String() {
			t.Fatalf("%s: %+v, %v; %+v", state, st, err, rec)
		}
	}
	for _, body := range []string{`{"state":"pending","created_at":"2030-01-01T00:00:00Z","expires_at":"2030-01-02T00:00:00Z"}`, `{"state":"ready","expires_at":"2030-01-02T00:00:00Z"}`} {
		c, _ := reqServer(t, http.StatusOK, body, nil)
		if _, err := c.RequestStatus(context.Background(), testID, testManage); !errors.Is(err, ErrProtocol) {
			t.Errorf("%s: err = %v", body, err)
		}
	}
	c, _ := reqServer(t, http.StatusNotFound, ``, nil)
	if _, err := c.RequestStatus(context.Background(), testID, testManage); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status 404 err = %v", err)
	}
	c, rec := reqServer(t, http.StatusNoContent, ``, nil)
	if err := c.CancelRequest(context.Background(), testID, testManage); err != nil || rec.method != http.MethodPost || rec.path != "/api/request/"+testID.String()+"/revoke" {
		t.Fatalf("cancel = %v; %+v", err, rec)
	}
	c, _ = reqServer(t, http.StatusNotFound, ``, nil)
	if err := c.CancelRequest(context.Background(), testID, testManage); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel 404 err = %v", err)
	}
}

// replyClaimHeaders are valid claim response headers for version.
func replyClaimHeaders(version string) http.Header {
	h := http.Header{}
	h.Set(headerVersion, version)
	h.Set(headerNonce, domain.EncodeB64URL(make([]byte, 12)))
	h.Set(headerClaim, testClaim.String())
	h.Set(headerClaimExpires, "2030-01-01T00:00:00Z")
	return h
}

func TestClaimAndAckReply(t *testing.T) {
	c, rec := reqServer(t, http.StatusOK, "ciphertext", replyClaimHeaders("3"))
	cl, err := c.ClaimReply(context.Background(), testID, testManage)
	if err != nil || string(cl.Body) != "ciphertext" || cl.Token != testClaim {
		t.Fatalf("ClaimReply = %+v, %v", cl, err)
	}
	if rec.path != "/api/request/"+testID.String()+"/reply" || rec.manage != testManage.String() || rec.method != http.MethodGet {
		t.Fatalf("claim request = %+v", rec)
	}
	// A secret version on the reply route, and v3 on the secret route, are refused.
	c, _ = reqServer(t, http.StatusOK, "x", replyClaimHeaders("1"))
	if _, err := c.ClaimReply(context.Background(), testID, testManage); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("v1 reply err = %v", err)
	}
	c, _ = reqServer(t, http.StatusOK, "x", replyClaimHeaders("3"))
	if _, err := c.Claim(context.Background(), testID, 3); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("v3 secret claim err = %v", err)
	}
	c, _ = reqServer(t, http.StatusNotFound, "", nil)
	if _, err := c.ClaimReply(context.Background(), testID, testManage); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim 404 err = %v", err)
	}
	c, rec = reqServer(t, http.StatusNoContent, "", nil)
	if err := c.AckReply(context.Background(), testID, testClaim); err != nil || rec.method != http.MethodDelete ||
		rec.path != "/api/request/"+testID.String()+"/reply" || rec.claim != testClaim.String() {
		t.Fatalf("ack = %v; %+v", err, rec)
	}
	c, _ = reqServer(t, http.StatusNotFound, "", nil)
	if err := c.AckReply(context.Background(), testID, testClaim); err != nil {
		t.Fatalf("ack 404 = %v, want nil", err)
	}
}
