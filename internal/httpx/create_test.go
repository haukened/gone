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

// newTestHandler returns a handler with the supplied maximum body size.
// It takes maxBody in bytes and returns a handler configured for create parsing tests.
func newTestHandler(maxBody int64) *Handler { return &Handler{MaxBody: maxBody} }

type secretHeaderCase struct {
	name    string
	headers [][2]string
	wantErr string
}

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
	const nonce = "AAAAAAAAAAAAAAAA"
	cases := []secretHeaderCase{
		{"ok", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, ""},
		{"missing all", nil, "missing required headers"},
		{"missing nonce", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-TTL", "5m"}}, "missing required headers"},
		{"empty ttl", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", ""}}, "missing required headers"},
		{"dup version", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, "missing required headers"},
		{"dup nonce", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, "missing required headers"},
		{"dup ttl", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}, {"X-Gone-TTL", "1h"}}, "missing required headers"},
		{"version too big", [][2]string{{"X-Gone-Version", "9999"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, "invalid version"},
		{"version leading zero", [][2]string{{"X-Gone-Version", "01"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, "invalid version"},
		{"ok v2", [][2]string{{"X-Gone-Version", "2"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, ""},
		{"version unsupported", [][2]string{{"X-Gone-Version", "3"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, "invalid version"},
		{"version signed", [][2]string{{"X-Gone-Version", "+1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "5m"}}, "invalid version"},
		{"nonce short", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", "n"}, {"X-Gone-TTL", "5m"}}, "invalid nonce"},
		{"nonce padded", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce + "=="}, {"X-Gone-TTL", "5m"}}, "invalid nonce"},
		{"nonce std alphabet", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", "AAAAAAAAAAAAAA+/"}, {"X-Gone-TTL", "5m"}}, "invalid nonce"},
		{"bad ttl", [][2]string{{"X-Gone-Version", "1"}, {"X-Gone-Nonce", nonce}, {"X-Gone-TTL", "notdur"}}, "invalid ttl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertParsedSecretHeaders(t, c, nonce)
		})
	}
}

// assertParsedSecretHeaders verifies parseSecretHeaders for one case.
// It takes t for failures, c as the case, and nonce as the expected nonce on success.
func assertParsedSecretHeaders(t *testing.T, c secretHeaderCase, nonce string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	for _, h := range c.headers {
		req.Header.Add(h[0], h[1])
	}
	ver, n, ttl, err := parseSecretHeaders(req)
	if c.wantErr == "" {
		assertSecretHeaderSuccess(t, c, ver, n, ttl, err, nonce)
		return
	}
	assertSecretHeaderError(t, c, err)
}

// assertSecretHeaderSuccess verifies parsed header values for a successful case.
// It takes t for failures, c as the case, parsed values, err, and nonce.
func assertSecretHeaderSuccess(t *testing.T, c secretHeaderCase, ver uint8, n string, ttl time.Duration, parseErr error, nonce string) {
	t.Helper()
	want, versionErr := strconv.Atoi(c.headers[0][1])
	if versionErr != nil {
		t.Fatalf("parse expected version: %v", versionErr)
	}
	if parseErr != nil || int(ver) != want || n != nonce || ttl != 5*time.Minute {
		t.Fatalf("unexpected parse: %v %d %s %v", parseErr, ver, n, ttl)
	}
}

// assertSecretHeaderError verifies the parse error and create-error classification.
// It takes t for failures, c as the case, and err as the parse result.
func assertSecretHeaderError(t *testing.T, c secretHeaderCase, err error) {
	t.Helper()
	if err == nil || err.Error() != c.wantErr {
		t.Fatalf("err = %v, want %q", err, c.wantErr)
	}
	if code, msg := classifyCreateError(err); code != http.StatusBadRequest || msg != c.wantErr {
		t.Fatalf("classify = %d %q", code, msg)
	}
}

func Test_parseAndValidateCreate(t *testing.T) {
	h := newTestHandler(50)
	req := httptest.NewRequest(http.MethodPost, "/api/secret", strings.NewReader("abc"))
	req.Header.Set("Content-Length", "3")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
	req.Header.Set("X-Gone-TTL", "1m")
	meta, err := h.parseAndValidateCreate(req)
	if err != nil || meta.contentLength != 3 || meta.version != 1 || meta.nonce != "AAAAAAAAAAAAAAAA" || meta.ttl != time.Minute {
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
