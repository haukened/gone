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

func (s stubTemplate) Execute(w http.ResponseWriter, data any) error {
	_, _ = w.Write([]byte(s.body))
	return nil
}

// errTemplate always returns an error to simulate template execution failure.
type errTemplate struct{}

func (e errTemplate) Execute(w http.ResponseWriter, data any) error {
	return errors.New("boom")
}

// TestHandleSecret covers routing logic, template success, template absence, and template failure.
func TestHandleSecret(t *testing.T) {
	tests := []struct {
		name               string
		path               string
		tmpl               SecretRenderer
		wantStatus         int
		wantBodyContains   string
		wantContentType    string
		wantCacheControl   string
		assertContentType  bool
		assertCacheControl bool
	}{
		{
			name:       "missing id with trailing slash",
			path:       "/secret/",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing id without trailing slash",
			path:       "/secret",
			wantStatus: http.StatusNotFound,
		},
		{
			name:             "nil template returns 503",
			path:             "/secret/abc123",
			tmpl:             nil,
			wantStatus:       http.StatusServiceUnavailable,
			wantBodyContains: "secret template unavailable",
			// content type not asserted: not explicitly set in handler
		},
		{
			name:               "successful template execution",
			path:               "/secret/abc123",
			tmpl:               stubTemplate{body: "<html>OK</html>"},
			wantStatus:         http.StatusOK,
			wantBodyContains:   "OK",
			wantContentType:    "text/html; charset=utf-8",
			wantCacheControl:   "no-store",
			assertContentType:  true,
			assertCacheControl: true,
		},
		{
			name:               "template execution error",
			path:               "/secret/abc123",
			tmpl:               errTemplate{},
			wantStatus:         http.StatusInternalServerError,
			wantBodyContains:   http.StatusText(http.StatusInternalServerError),
			wantContentType:    "text/plain; charset=utf-8",
			wantCacheControl:   "no-store",
			assertContentType:  true,
			assertCacheControl: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{
				SecretTmpl: tc.tmpl,
			}
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()

			h.handleSecret(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("unexpected status: got %d want %d; body=%q", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantBodyContains != "" && !strings.Contains(rr.Body.String(), tc.wantBodyContains) {
				t.Fatalf("body %q does not contain %q", rr.Body.String(), tc.wantBodyContains)
			}
			if tc.assertContentType {
				gotCT := rr.Header().Get("Content-Type")
				if gotCT != tc.wantContentType {
					t.Fatalf("content-type mismatch: got %q want %q", gotCT, tc.wantContentType)
				}
			}
			if tc.assertCacheControl {
				gotCC := rr.Header().Get("Cache-Control")
				if gotCC != tc.wantCacheControl {
					t.Fatalf("cache-control mismatch: got %q want %q", gotCC, tc.wantCacheControl)
				}
			}
		})
	}
}
