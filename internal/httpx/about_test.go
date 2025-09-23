package httpx_test

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

type noopServiceAbout struct{}

func (noopServiceAbout) CreateSecret(context.Context, io.Reader, int64, uint8, string, time.Duration) (domain.SecretID, time.Time, error) {
	return domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), time.Now().Add(time.Hour), nil
}
func (noopServiceAbout) Consume(context.Context, string) (app.Meta, io.ReadCloser, int64, error) {
	return app.Meta{Version: 1, NonceB64u: "n"}, io.NopCloser(io.Reader(nil)), 0, nil
}

func TestAboutHandler(t *testing.T) {
	tmpl := template.Must(template.New("about").Parse(`<html><body><h2>How Gone Keeps Secrets Secret</h2></body></html>`))
	h := httpx.New(noopServiceAbout{}, 100, nil)
	h.AboutTmpl = httpx.AboutTemplateRenderer{T: tmpl}
	r := httptest.NewRequest(http.MethodGet, "/about", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content-type %s", ct)
	}
	if body := w.Body.String(); !containsAll(body, []string{"How Gone Keeps Secrets Secret"}) {
		t.Fatalf("missing expected about content: %s", body)
	}
}

// containsAll helper to avoid repeating strings.Contains checks.
func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
