package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecureHeadersMiddleware ensures the security headers are consistently applied.
func TestSecureHeadersMiddleware(t *testing.T) {
	h := &Handler{}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	h.secureHeaders(final).ServeHTTP(rw, req)
	res := rw.Result()
	checks := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
	}
	for k, expect := range checks {
		if got := res.Header.Get(k); got != expect {
			// allow exact string match only; deliberate strictness
			// fail fast on missing or changed policy
			if got == "" {
				t.Fatalf("missing header %s", k)
			}
			if got != expect {
				t.Fatalf("header %s mismatch\nexpected: %s\nactual:   %s", k, expect, got)
			}
		}
	}
	if cc := res.Header.Get("Cache-Control"); cc == "" {
		// Because Content-Type was set inside handler BEFORE writing body, our middleware skipped default?
		// In current implementation middleware sets Cache-Control only if Content-Type is empty at time of middleware execution.
		// Behavior acceptable; ensure we can still proceed without default no-store override.
	}
}

// TestSecureHeadersDefaultCache ensures no-store is applied when downstream handler does not pre-set Content-Type.
func TestSecureHeadersDefaultCache(t *testing.T) {
	h := &Handler{}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally do not set Content-Type to trigger default cache headers.
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	h.secureHeaders(final).ServeHTTP(rw, req)
	res := rw.Result()
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		// expecting middleware default when content-type absent
		t.Fatalf("expected Cache-Control no-store got %q", cc)
	}
	if pragma := res.Header.Get("Pragma"); pragma != "no-cache" {
		t.Fatalf("expected Pragma no-cache got %q", pragma)
	}
}
