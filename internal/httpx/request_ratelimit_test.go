package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequestRateLimitScopes checks that sending a reply spends the create
// budget while every other request route spends the read budget.
func TestRequestRateLimitScopes(t *testing.T) {
	id := strings.Repeat("ab", 16)
	cases := []struct {
		method, path string
		create       bool
	}{
		{http.MethodPut, "/api/request/" + id + "/reply", true},
		{http.MethodPost, "/api/request", true},
		{http.MethodGet, "/api/request/" + id + "/reply", false},
		{http.MethodGet, "/api/request/" + id + "/status", false},
		{http.MethodGet, "/api/request/" + id, false},
		{http.MethodPost, "/api/request/" + id + "/revoke", false},
	}
	for _, c := range cases {
		create, read := &stubLimiter{}, &stubLimiter{}
		m := &countingMetrics{}
		h := &Handler{CreateLimiter: create, ReadLimiter: read, Metrics: m}
		w := httptest.NewRecorder()
		h.Router().ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("%s %s: code=%d", c.method, c.path, w.Code)
		}
		if (create.calls == 1) != c.create || (read.calls == 1) == c.create {
			t.Errorf("%s %s: create=%d read=%d calls", c.method, c.path, create.calls, read.calls)
		}
	}
	// No limiter configured for the chosen scope admits the request.
	h := &Handler{CreateLimiter: &stubLimiter{}}
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/request/"+id+"/status", nil))
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("read route limited without a read limiter")
	}
}
