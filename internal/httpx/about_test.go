package httpx

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubAboutErr always errors.
type stubAboutErr struct{}

func (stubAboutErr) Execute(_ http.ResponseWriter, _ any) error { return errors.New("boom") }

// aboutTemplateRenderer constructs a basic template renderer similar to production for integration-like success case.
func aboutTemplateRenderer() AboutTemplateRenderer {
	tmpl := template.Must(template.New("about").Parse(`<html><body><h2>How Gone Keeps Secrets Secret</h2></body></html>`))
	return AboutTemplateRenderer{T: tmpl}
}

func TestHandleAbout_AllBranches(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		tmpl          AboutRenderer
		direct        bool // call handleAbout directly (avoids mux routing differences)
		wantStatus    int
		wantContains  []string
		wantCT        string
		wantCacheCtrl string
	}{
		{name: "wrong path", path: "/aboutx", tmpl: aboutTemplateRenderer(), direct: true, wantStatus: http.StatusNotFound},
		{name: "nil template", path: "/about", tmpl: nil, direct: true, wantStatus: http.StatusServiceUnavailable, wantContains: []string{"about unavailable"}},
		{name: "template error", path: "/about", tmpl: stubAboutErr{}, direct: true, wantStatus: http.StatusInternalServerError, wantContains: []string{http.StatusText(http.StatusInternalServerError)}, wantCT: "text/plain; charset=utf-8"},
		{name: "success", path: "/about", tmpl: aboutTemplateRenderer(), direct: true, wantStatus: http.StatusOK, wantContains: []string{"How Gone Keeps Secrets Secret"}, wantCT: "text/html; charset=utf-8", wantCacheCtrl: "no-store"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertAboutCase(t, tc.path, tc.tmpl, tc.wantStatus, tc.wantContains, tc.wantCT, tc.wantCacheCtrl)
		})
	}
}

// assertAboutCase executes handleAbout and verifies its response.
// It takes t for failures, path and tmpl for the request, status, body substrings, content type, and cache control expectations.
func assertAboutCase(t *testing.T, path string, tmpl AboutRenderer, wantStatus int, wantContains []string, wantCT, wantCacheCtrl string) {
	t.Helper()
	h := &Handler{AboutTmpl: tmpl}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	h.handleAbout(rr, req)
	if rr.Code != wantStatus {
		t.Fatalf("status %d want %d body=%q", rr.Code, wantStatus, rr.Body.String())
	}
	assertAboutBody(t, rr.Body.String(), wantContains)
	assertAboutOptionalHeader(t, rr, "Content-Type", wantCT)
	assertAboutOptionalHeader(t, rr, "Cache-Control", wantCacheCtrl)
}

// assertAboutBody verifies all expected substrings are present in the about response body.
// It takes t for failures, body as the response text, and wantContains as required substrings.
func assertAboutBody(t *testing.T, body string, wantContains []string) {
	t.Helper()
	for _, sub := range wantContains {
		if !strings.Contains(body, sub) {
			t.Fatalf("body missing %q: %s", sub, body)
		}
	}
}

// assertAboutOptionalHeader verifies a header only when a non-empty expected value is supplied.
// It takes t for failures, rr as the response, name as the header name, and want as the expected value.
func assertAboutOptionalHeader(t *testing.T, rr *httptest.ResponseRecorder, name, want string) {
	t.Helper()
	if want == "" {
		return
	}
	if got := rr.Header().Get(name); got != want {
		t.Fatalf("%s %q want %q", name, got, want)
	}
}
