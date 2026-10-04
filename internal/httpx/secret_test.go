package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubTemplate is a successful template that writes a fixed body.
type stubTemplate struct {
	body string
}

// Execute writes the configured body and returns no error.
// It takes w as the destination and ignores template data, returning any write error.
func (s stubTemplate) Execute(w http.ResponseWriter, _ any) error {
	_, _ = w.Write([]byte(s.body))
	return nil
}

// errTemplate always returns an error to simulate template execution failure.
type errTemplate struct{}

// Execute returns a fixed error without writing a response.
// It ignores the response writer and template data, and returns the simulated error.
func (e errTemplate) Execute(_ http.ResponseWriter, _ any) error {
	return errors.New("boom")
}

type secretHandlerCase struct {
	name               string
	path               string
	tmpl               SecretRenderer
	wantStatus         int
	wantBodyContains   string
	wantContentType    string
	wantCacheControl   string
	assertContentType  bool
	assertCacheControl bool
}

var secretHandlerCases = []secretHandlerCase{
	{name: "missing id with trailing slash", path: "/secret/", wantStatus: http.StatusNotFound},
	{name: "missing id without trailing slash", path: "/secret", wantStatus: http.StatusNotFound},
	{name: "nil template returns 503", path: "/secret/abc123", tmpl: nil, wantStatus: http.StatusServiceUnavailable, wantBodyContains: "secret template unavailable"},
	{name: "successful template execution", path: "/secret/abc123", tmpl: stubTemplate{body: "<html>OK</html>"}, wantStatus: http.StatusOK, wantBodyContains: "OK", wantContentType: "text/html; charset=utf-8", wantCacheControl: "no-store", assertContentType: true, assertCacheControl: true},
	{name: "template execution error", path: "/secret/abc123", tmpl: errTemplate{}, wantStatus: http.StatusInternalServerError, wantBodyContains: http.StatusText(http.StatusInternalServerError), wantContentType: "text/plain; charset=utf-8", wantCacheControl: "no-store", assertContentType: true, assertCacheControl: true},
}

// TestHandleSecret covers routing logic, template success, template absence, and template failure.
func TestHandleSecret(t *testing.T) {
	for _, tc := range secretHandlerCases {
		t.Run(tc.name, func(t *testing.T) {
			assertSecretHandlerCase(t, tc)
		})
	}
}

// assertSecretHandlerCase executes and verifies one handleSecret case.
// It takes t for failures and tc as the scenario under test.
func assertSecretHandlerCase(t *testing.T, tc secretHandlerCase) {
	t.Helper()
	h := &Handler{SecretTmpl: tc.tmpl}
	req := httptest.NewRequest(http.MethodGet, tc.path, nil)
	rr := httptest.NewRecorder()
	h.handleSecret(rr, req)
	if rr.Code != tc.wantStatus {
		t.Fatalf("unexpected status: got %d want %d; body=%q", rr.Code, tc.wantStatus, rr.Body.String())
	}
	if tc.wantBodyContains != "" && !strings.Contains(rr.Body.String(), tc.wantBodyContains) {
		t.Fatalf("body %q does not contain %q", rr.Body.String(), tc.wantBodyContains)
	}
	assertSecretHandlerHeader(t, rr, "Content-Type", tc.wantContentType, tc.assertContentType)
	assertSecretHandlerHeader(t, rr, "Cache-Control", tc.wantCacheControl, tc.assertCacheControl)
}

// assertSecretHandlerHeader verifies one optional response header expectation.
// It takes t for failures, rr as the response, name and want as the header expectation, and enabled.
func assertSecretHandlerHeader(t *testing.T, rr *httptest.ResponseRecorder, name, want string, enabled bool) {
	t.Helper()
	if !enabled {
		return
	}
	if got := rr.Header().Get(name); got != want {
		t.Fatalf("%s mismatch: got %q want %q", name, got, want)
	}
}
