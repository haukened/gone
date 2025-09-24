package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Test_checkMethodPath covers allowed and disallowed methods/paths.
func Test_checkMethodPath(t *testing.T) {
	tests := []struct {
		method, path string
		wantErr      bool
	}{
		{http.MethodPost, "/api/secret", false},
		{http.MethodGet, "/api/secret", true},
		{http.MethodPost, "/api/secret/", true},
		{http.MethodPost, "/api/secretx", true},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		err := checkMethodPath(req)
		if (err != nil) != tc.wantErr {
			t.Fatalf("method=%s path=%s wantErr=%v got %v", tc.method, tc.path, tc.wantErr, err)
		}
	}
}

// helper handler with configurable MaxBody
func newTestHandler(max int64) *Handler { return &Handler{MaxBody: max} }

func Test_parseContentLength(t *testing.T) {
	h := newTestHandler(10)
	// valid
	req := httptest.NewRequest(http.MethodPost, "/api/secret", strings.NewReader("12345"))
	req.Header.Set("Content-Length", "5")
	v, err := h.parseContentLength(req)
	if err != nil || v != 5 {
		t.Fatalf("expected 5 got %d err %v", v, err)
	}
	// missing
	req2 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	if _, err := h.parseContentLength(req2); err == nil {
		t.Fatalf("expected error for missing content-length")
	}
	// invalid number
	req3 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	req3.Header.Set("Content-Length", "abc")
	if _, err := h.parseContentLength(req3); err == nil {
		t.Fatalf("expected parse error")
	}
	// zero
	req4 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	req4.Header.Set("Content-Length", "0")
	if _, err := h.parseContentLength(req4); err == nil {
		t.Fatalf("expected zero error")
	}
	// exceeded
	req5 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	req5.Header.Set("Content-Length", strconv.FormatInt(11, 10))
	if _, err := h.parseContentLength(req5); err == nil {
		t.Fatalf("expected exceeded error")
	}
}

func Test_parseSecretHeaders(t *testing.T) {
	// success
	req := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "n")
	req.Header.Set("X-Gone-TTL", "5m")
	ver, nonce, ttl, err := parseSecretHeaders(req)
	if err != nil || ver != 1 || nonce != "n" || ttl != 5*time.Minute {
		t.Fatalf("unexpected success parse: %v %d %s %v", err, ver, nonce, ttl)
	}
	// missing
	req2 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	if _, _, _, err := parseSecretHeaders(req2); err == nil {
		t.Fatalf("expected missing headers error")
	}
	// bad version
	req3 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	req3.Header.Set("X-Gone-Version", "9999")
	req3.Header.Set("X-Gone-Nonce", "n")
	req3.Header.Set("X-Gone-TTL", "5m")
	if _, _, _, err := parseSecretHeaders(req3); err == nil {
		t.Fatalf("expected invalid version error")
	}
	// bad ttl
	req4 := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	req4.Header.Set("X-Gone-Version", "1")
	req4.Header.Set("X-Gone-Nonce", "n")
	req4.Header.Set("X-Gone-TTL", "notdur")
	if _, _, _, err := parseSecretHeaders(req4); err == nil {
		t.Fatalf("expected invalid ttl error")
	}
}

func Test_parseAndValidateCreate(t *testing.T) {
	h := newTestHandler(50)
	req := httptest.NewRequest(http.MethodPost, "/api/secret", strings.NewReader("abc"))
	req.Header.Set("Content-Length", "3")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "n")
	req.Header.Set("X-Gone-TTL", "1m")
	meta, err := h.parseAndValidateCreate(req)
	if err != nil || meta.contentLength != 3 || meta.version != 1 || meta.nonce != "n" || meta.ttl != time.Minute {
		t.Fatalf("unexpected meta %+v err %v", meta, err)
	}
	// method error
	bad := httptest.NewRequest(http.MethodGet, "/api/secret", nil)
	if _, err := h.parseAndValidateCreate(bad); err == nil {
		t.Fatalf("expected method error")
	}
}

func Test_classifyCreateError(t *testing.T) {
	cases := []string{"method not allowed", "not found", "content length required", "invalid content length", "size exceeded", "missing required headers", "invalid version", "invalid ttl", "other"}
	for _, c := range cases {
		code, msg := classifyCreateError(errors.New(c))
		if c == "other" {
			if code != http.StatusBadRequest || msg != "bad request" {
				t.Fatalf("unexpected default mapping %d %s", code, msg)
			}
			continue
		}
		if msg != c {
			t.Fatalf("expected msg %s got %s", c, msg)
		}
		if code == 0 {
			t.Fatalf("expected non-zero code for %s", c)
		}
	}
}
