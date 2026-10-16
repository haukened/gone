package integration_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
	"github.com/haukened/gone/v3/internal/httpx"
)

// TestReplyRowsUnreachableThroughSecretRoute checks that a request's reply,
// once stored, cannot be acknowledged, inspected or revoked through the
// /api/secret/{id} routes (it is a different resource), and that the
// /api/request/{id} routes still work on it afterwards.
func TestReplyRowsUnreachableThroughSecretRoute(t *testing.T) {
	srv := newRequestServer(t)
	priv, err := envelope.NewRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	c := requestCreate(t, srv.URL)
	if code := requestReply(t, srv.URL, c, priv.PublicKey().Bytes(), []byte("reply")); code != http.StatusCreated {
		t.Fatalf("reply = %d", code)
	}
	claim := requestDo(t, http.MethodGet, srv.URL+"/api/request/"+c.ID+"/reply", nil, httpx.HeaderManage, c.ManageToken)
	if claim.code != http.StatusOK {
		t.Fatalf("claim = %d", claim.code)
	}
	token := claim.header.Get(httpx.HeaderClaim)
	secret := srv.URL + "/api/secret/" + c.ID
	for _, tc := range []struct {
		name, method, url string
		headers           []string
	}{
		{"ack with the reply's claim token", http.MethodDelete, secret, []string{httpx.HeaderClaim, token}},
		{"status with the request's manage token", http.MethodGet, secret + "/status", []string{httpx.HeaderManage, c.ManageToken}},
		{"revoke with the request's manage token", http.MethodPost, secret + "/revoke", []string{httpx.HeaderManage, c.ManageToken}},
	} {
		if resp := requestDo(t, tc.method, tc.url, nil, tc.headers...); resp.code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", tc.name, resp.code)
		}
	}
	if got := requestState(t, srv.URL, c); got != "ready" {
		t.Fatalf("reply state after refused secret-route calls = %q, want ready", got)
	}
	ack := requestDo(t, http.MethodDelete, srv.URL+"/api/request/"+c.ID+"/reply", nil, httpx.HeaderClaim, token)
	if ack.code != http.StatusNoContent {
		t.Fatalf("request-route ack = %d, want 204", ack.code)
	}
}

// TestShortBodyIs400 checks a body shorter than its Content-Length is a 400
// with the real store, for both an inline and an external-size payload.
func TestShortBodyIs400(t *testing.T) {
	h := newRequestHandler(t)
	cfg := config.DefaultAppConfig
	for name, size := range map[string]int64{"inline": 10, "external": cfg.InlineMaxBytes + 1} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/secret", strings.NewReader("abc"))
			req.Header.Set("Content-Length", strconv.FormatInt(size, 10))
			req.Header.Set("X-Gone-Version", "1")
			req.Header.Set("X-Gone-Nonce", domain.EncodeB64URL(make([]byte, domain.NonceSize)))
			req.Header.Set("X-Gone-TTL", "5m")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"short body"`) {
				t.Fatalf("got %d %s, want 400 short body", rr.Code, rr.Body.String())
			}
		})
	}
}
