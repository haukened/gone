package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/haukened/gone/internal/httpx"
)

// static error tests rely on noopService from existing tests.

func TestStaticHandlerErrors(t *testing.T) {
	dir := t.TempDir()
	// create a real file with extension to ensure handler is mounted
	if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	h := httpx.New(noopService{}, 0, nil)
	h.Assets = http.FS(os.DirFS(dir))
	router := h.Router()

	t.Run("directory path 404", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/static/", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 got %d", w.Code)
		}
	})

	t.Run("missing extension 404", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/static/style", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 got %d", w.Code)
		}
	})
}

// TestStaticHandlerPinnedTypes verifies assets whose types minimal container
// images may not know (.woff2, .ico) are served with a fixed Content-Type, so
// nosniff never blocks them.
func TestStaticHandlerPinnedTypes(t *testing.T) {
	cases := []struct{ file, want string }{
		{"f.woff2", "font/woff2"},
		{"favicon.ico", "image/x-icon"},
	}
	dir := t.TempDir()
	for _, tc := range cases {
		if err := os.WriteFile(filepath.Join(dir, tc.file), []byte("data"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	h := httpx.New(noopService{}, 0, nil)
	h.Assets = http.FS(os.DirFS(dir))
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/static/"+tc.file, nil)
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 got %d", w.Code)
			}
			if got := w.Header().Get("Content-Type"); got != tc.want {
				t.Fatalf("expected %s got %q", tc.want, got)
			}
		})
	}
}
