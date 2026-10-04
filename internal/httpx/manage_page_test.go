package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/httpx"
)

// pageTemplate is a SecretRenderer that writes a fixed body.
type pageTemplate struct{ body string }

// Execute implements httpx.SecretRenderer.
func (p pageTemplate) Execute(w http.ResponseWriter, _ any) error {
	_, err := w.Write([]byte(p.body))
	return err
}

func TestHandleManagePage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		tmpl     httpx.SecretRenderer
		wantCode int
		wantBody string
	}{
		{"renders page", "/manage/" + manageID, pageTemplate{body: "manage-page"}, http.StatusOK, "manage-page"},
		{"bare prefix", "/manage/", pageTemplate{body: "manage-page"}, http.StatusNotFound, "not found"},
		{"nil template", "/manage/" + manageID, nil, http.StatusServiceUnavailable, "manage template unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := httpx.New(mockService{}, 1024, nil)
			h.ManageTmpl = tc.tmpl
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("status=%d want %d", w.Code, tc.wantCode)
			}
			if !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("body %q missing %q", w.Body.String(), tc.wantBody)
			}
		})
	}
}
