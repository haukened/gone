package httpx_test

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haukened/gone/internal/httpx"
)

func TestHealthAndReady(t *testing.T) {
	readyCalled := false
	readiness := func(context.Context) error { readyCalled = true; return nil }
	h := httpx.New(mockService{}, 10, readiness)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health status %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ready status %d", w.Code)
	}
	if !readyCalled {
		t.Fatalf("readiness not invoked")
	}
}
func TestHandleSecretPage(t *testing.T) {
	// provide a minimal secret template
	tmpl := template.Must(template.New("secret").Parse(`<!DOCTYPE html><html><body>{{template "header" .}}<div id="view-open"></div></body></html>`))
	h := httpx.New(mockService{}, 1024, nil)
	// Need partials header template to satisfy reference; keep it simple
	tmplWithPartials := template.Must(template.New("partials").Parse(`{{define "header"}}<header>H</header>{{end}}`))
	tmplWithPartials = template.Must(tmplWithPartials.AddParseTree("secret", tmpl.Tree))
	h.SecretTmpl = httpx.TemplateRenderer{T: tmplWithPartials}
	req := httptest.NewRequest(http.MethodGet, "/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type %s", ct)
	}
}
