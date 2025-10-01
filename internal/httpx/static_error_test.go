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
