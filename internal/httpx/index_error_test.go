package httpx_test

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haukened/gone/internal/httpx"
)

// failingRenderer simulates a template execution failure.
type failingRenderer struct{}

func (f failingRenderer) Execute(w http.ResponseWriter, _ any) error {
	return errors.New("template boom")
}

// reuse noopService from index_test.go (same package httpx_test)

func TestIndexHandlerErrors(t *testing.T) {
	// 1. JSON 404 path mismatch: request /api/unknown -> expect JSON {"error":"not found"}
	t.Run("api path mismatch json 404", func(t *testing.T) {
		h := httpx.New(noopService{}, 0, nil)
		req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected JSON content-type got %s", ct)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("not found")) {
			t.Fatalf("expected body to contain not found got %s", rec.Body.String())
		}
	})

	// 2. Nil template -> 503
	t.Run("nil index template 503", func(t *testing.T) {
		h := httpx.New(noopService{}, 0, nil)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 got %d", rec.Code)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("index unavailable")) {
			t.Fatalf("expected placeholder body got %s", rec.Body.String())
		}
	})

	// 3. Template execution failure -> 500 plain text "template error"
	t.Run("template exec error 500", func(t *testing.T) {
		goodTmpl := template.Must(template.New("ok").Parse("ok"))
		h := httpx.New(noopService{}, 0, nil)
		h.IndexTmpl = failingRenderer{}
		// Also set ErrorTmpl to ensure it doesn't interfere; index path uses IndexTmpl directly.
		h.ErrorTmpl = httpx.TemplateRenderer{T: goodTmpl}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 got %d body=%s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("expected plain text content-type got %s", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != http.StatusText(http.StatusInternalServerError) {
			t.Fatalf("expected body %q got %q", http.StatusText(http.StatusInternalServerError), rec.Body.String())
		}
	})
}
