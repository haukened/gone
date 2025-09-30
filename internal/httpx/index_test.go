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
	tmpl := template.Must(template.New("index").Parse(`<html><body><p>{{ .MaxBytes }}</p>{{ range .TTLOptions }}<option>{{ .Label }}</option>{{ end }}</body></html>`))
	h := httpx.New(noopService{}, 1234, nil)
	h.IndexTmpl = httpx.TemplateRenderer{T: tmpl}
	h.MinTTL = 5 * time.Minute
	h.MaxTTL = 60 * time.Minute
	h.TTLOptions = []domain.TTLOption{{Duration: 5 * time.Minute, Label: "5m"}, {Duration: 30 * time.Minute, Label: "30m"}, {Duration: time.Hour, Label: "1h"}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type %s", ct)
	}
	body := w.Body.String()
	// presence checks
	for _, expect := range []string{"1234", "5m", "30m", "1h"} {
		if !strings.Contains(body, expect) {
			t.Fatalf("expected body to contain %q: %s", expect, body)
		}
	}
	// order check: longest first
	idx1h := strings.Index(body, ">1h<")
	idx30m := strings.Index(body, ">30m<")
	idx5m := strings.Index(body, ">5m<")
	if !(idx1h != -1 && idx30m != -1 && idx5m != -1 && idx1h < idx30m && idx30m < idx5m) {
		t.Fatalf("expected descending order 1h,30m,5m; got positions %d %d %d in %s", idx1h, idx30m, idx5m, body)
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

// TestNotFoundHTML ensures non-API unknown routes return an HTML 404 page (not JSON).
func TestNotFoundHTML(t *testing.T) {
	indexTmpl := template.Must(template.New("index").Parse(`<html><body>Index</body></html>`))
	errorTmpl := template.Must(template.New("error").Parse(`<!DOCTYPE html><html><body>Error {{ .Status }} - {{ .Title }} :: {{ .Message }}</body></html>`))
	h := httpx.New(noopService{}, 100, nil)
	h.IndexTmpl = httpx.TemplateRenderer{T: indexTmpl}
	h.ErrorTmpl = httpx.TemplateRenderer{T: errorTmpl}
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected html content-type got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Error 404") || strings.Contains(body, `{"error"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}
