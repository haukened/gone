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

// TestStaticHandlerFontType verifies .woff2 assets are served as font/woff2
// regardless of the host MIME table, so nosniff never blocks the font.
func TestStaticHandlerFontType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.woff2"), []byte("wOF2"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	h := httpx.New(noopService{}, 0, nil)
	h.Assets = http.FS(os.DirFS(dir))
	r := httptest.NewRequest(http.MethodGet, "/static/f.woff2", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "font/woff2" {
		t.Fatalf("expected font/woff2 got %q", got)
	}
}
