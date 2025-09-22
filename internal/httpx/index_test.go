package httpx_test

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

type noopService struct{}

func (noopService) CreateSecret(_ context.Context, _ io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (domain.SecretID, time.Time, error) {
	return domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), time.Now().Add(time.Hour), nil
}
func (noopService) Consume(_ context.Context, _ string) (app.Meta, io.ReadCloser, int64, error) {
	return app.Meta{Version: 1, NonceB64u: "n"}, io.NopCloser(bytes.NewReader([]byte("x"))), 1, nil
}

// TestIndexHandler ensures the index template renders and headers are set.
func TestIndexHandler(t *testing.T) {
	tmpl := template.Must(template.New("index").Parse(`<html><body><p>{{ .MaxBytes }}</p></body></html>`))
	h := httpx.New(noopService{}, 1234, nil)
	h.IndexTmpl = httpx.TemplateRenderer{T: tmpl}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type %s", ct)
	}
	if !strings.Contains(w.Body.String(), "1234") {
		t.Fatalf("missing max bytes: %s", w.Body.String())
	}
}

// TestStaticHandler ensures static file caching header is set.
func TestStaticHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("ok"), 0o600); err != nil {
		panic(err)
	}
	fs := http.FS(os.DirFS(dir))
	h := httpx.New(noopService{}, 100, nil)
	h.Assets = fs
	r := httptest.NewRequest(http.MethodGet, "/static/test.txt", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		panic(w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc == "" {
		panic("missing cache-control")
	}
}
