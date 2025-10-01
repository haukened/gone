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

func (stubAboutErr) Execute(w http.ResponseWriter, data any) error { return errors.New("boom") }

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
			h := &Handler{AboutTmpl: tc.tmpl}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			h.handleAbout(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status %d want %d body=%q", rr.Code, tc.wantStatus, rr.Body.String())
			}
			body := rr.Body.String()
			for _, sub := range tc.wantContains {
				if !strings.Contains(body, sub) {
					t.Fatalf("body missing %q: %s", sub, body)
				}
			}
			if tc.wantCT != "" {
				if got := rr.Header().Get("Content-Type"); got != tc.wantCT {
					t.Fatalf("content-type %q want %q", got, tc.wantCT)
				}
			}
			if tc.wantCacheCtrl != "" {
				if got := rr.Header().Get("Cache-Control"); got != tc.wantCacheCtrl {
					t.Fatalf("cache-control %q want %q", got, tc.wantCacheCtrl)
				}
			}
		})
	}
}
