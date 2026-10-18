package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/haukened/gone/v3/internal/auth"
	"github.com/haukened/gone/v3/internal/metrics"
)

// signInHandler returns a handler whose sign-in trusts X-Gone-User from
// httptest's default peer, 192.0.2.1.
func signInHandler(t *testing.T) (*Handler, *countingMetrics) {
	t.Helper()
	trusted := []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")}
	s := auth.Settings{Provider: auth.Custom, Header: "X-Gone-User", SignInURL: "/login?rd={return}"}
	if _, err := s.Validate(true); err != nil {
		t.Fatal(err)
	}
	m := &countingMetrics{}
	h := &Handler{Service: &recordingService{}, MaxBody: 1024, Auth: auth.New(s, trusted, nil), Metrics: m}
	return h, m
}

func TestRequireSignIn(t *testing.T) {
	h, m := signInHandler(t)
	reached := 0
	next := func(w http.ResponseWriter, _ *http.Request) { reached++; w.WriteHeader(http.StatusTeapot) }
	wrapped := h.requireSignIn(next)

	w := httptest.NewRecorder()
	wrapped(w, httptest.NewRequest(http.MethodPost, "/api/secret", bytes.NewReader(nil)))
	if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != `Bearer realm="gone"` || reached != 0 {
		t.Fatalf("unsigned POST: %d %q reached=%d", w.Code, w.Header().Get("WWW-Authenticate"), reached)
	}
	var body struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error != "sign in required" {
		t.Fatalf("body = %s", w.Body)
	}
	if m.counts[metrics.CounterSignInDenied] != 1 {
		t.Fatalf("counter = %d", m.counts[metrics.CounterSignInDenied])
	}

	signed := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	signed.Header.Set("X-Gone-User", "alice")
	w = httptest.NewRecorder()
	wrapped(w, signed)
	if w.Code != http.StatusTeapot || reached != 1 {
		t.Fatalf("signed POST: %d reached=%d", w.Code, reached)
	}

	w = httptest.NewRecorder()
	wrapped(w, httptest.NewRequest(http.MethodGet, "/api/secret", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("GET should reach the handler for its own 405, got %d", w.Code)
	}

	h.Auth = nil
	w = httptest.NewRecorder()
	wrapped(w, httptest.NewRequest(http.MethodPost, "/api/secret", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("sign-in off: %d", w.Code)
	}
}

func TestSignInRoutes(t *testing.T) {
	h, _ := signInHandler(t)
	router := h.Router()
	for _, path := range []string{"/api/secret", "/api/request"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("POST %s unsigned = %d, want 401", path, w.Code)
		}
	}
	for _, path := range []string{"/api/secret/0123456789abcdef0123456789abcdef", "/api/policy", "/healthz"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code == http.StatusUnauthorized {
			t.Errorf("GET %s needs sign-in", path)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/policy", nil))
	var p struct {
		SignInRequired bool `json:"sign_in_required"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || !p.SignInRequired {
		t.Fatalf("policy = %s", w.Body)
	}
}

// TestSignInLeavesPublicRoutesOpen checks opening, status, revoking and
// replying never need sign-in: only creating does. Any status but 401 is
// fine here; the handlers' own checks are tested elsewhere.
func TestSignInLeavesPublicRoutesOpen(t *testing.T) {
	h, m := signInHandler(t)
	router := h.Router()
	const id = "0123456789abcdef0123456789abcdef"
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/secret/" + id},
		{http.MethodDelete, "/api/secret/" + id},
		{http.MethodGet, "/api/secret/" + id + "/status"},
		{http.MethodPost, "/api/secret/" + id + "/revoke"},
		{http.MethodGet, "/api/request/" + id},
		{http.MethodPut, "/api/request/" + id + "/reply"},
		{http.MethodGet, "/api/request/" + id + "/status"},
		{http.MethodPost, "/api/request/" + id + "/revoke"},
	}
	for _, rt := range routes {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(rt.method, rt.path, bytes.NewReader([]byte("x"))))
		if w.Code == http.StatusUnauthorized {
			t.Errorf("%s %s needs sign-in", rt.method, rt.path)
		}
	}
	if n := m.counts[metrics.CounterSignInDenied]; n != 0 {
		t.Fatalf("sign_in_denied_total = %d", n)
	}
}

func TestNeedsSignIn(t *testing.T) {
	h, _ := signInHandler(t)
	need, url := h.needsSignIn(httptest.NewRequest(http.MethodGet, "/?rd=https://evil.example", nil), "/request")
	if !need || url != "/login?rd=%2Frequest" {
		t.Fatalf("unsigned: %v %q", need, url)
	}
	signed := httptest.NewRequest(http.MethodGet, "/", nil)
	signed.Header.Set("X-Gone-User", "alice")
	if need, _ := h.needsSignIn(signed, "/"); need {
		t.Fatal("a signed-in visitor sees the sign-in view")
	}
	h.Auth = nil
	if need, url := h.needsSignIn(httptest.NewRequest(http.MethodGet, "/", nil), "/"); need || url != "" {
		t.Fatal("sign-in off shows the view")
	}
}
