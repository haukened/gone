package httpx_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

type mockService struct {
	createFn  func(ctx context.Context, ct io.Reader, size int64, _ uint8, _ string, _ time.Duration) (domain.SecretID, time.Time, error)
	consumeFn func(ctx context.Context, id string) (app.Meta, io.ReadCloser, int64, error)
}

func (m mockService) CreateSecret(ctx context.Context, ct io.Reader, size int64, version uint8, nonce string, ttl time.Duration) (domain.SecretID, time.Time, error) {
	return m.createFn(ctx, ct, size, version, nonce, ttl)
}
func (m mockService) Consume(ctx context.Context, idStr string) (app.Meta, io.ReadCloser, int64, error) {
	return m.consumeFn(ctx, idStr)
}

func TestHandleCreateSecretSuccess(t *testing.T) {
	m := mockService{createFn: func(_ context.Context, ct io.Reader, size int64, _ uint8, _ string, _ time.Duration) (domain.SecretID, time.Time, error) {
		b, _ := io.ReadAll(ct)
		if string(b) != "cipher" {
			t.Fatalf("unexpected body")
		}
		if size != int64(len(b)) {
			t.Fatalf("size mismatch")
		}
		return domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), time.Unix(1000, 0).UTC(), nil
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/secret", bytes.NewReader([]byte("cipher")))
	req.Header.Set("Content-Length", "6")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "n1")
	req.Header.Set("X-Gone-TTL", "5m")
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %s", ct)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")) {
		t.Fatalf("missing id")
	}
}

func TestHandleCreateSecretValidationErrors(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/secret", bytes.NewReader([]byte("cipher")))
	// Intentionally omit Content-Length
	h := httpx.New(mockService{createFn: func(_ context.Context, _ io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (domain.SecretID, time.Time, error) {
		return "", time.Time{}, nil
	}}, 10, nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusLengthRequired {
		t.Fatalf("expected 411 got %d", w.Code)
	}
}

func TestHandleConsumeSuccess(t *testing.T) {
	m := mockService{consumeFn: func(_ context.Context, id string) (app.Meta, io.ReadCloser, int64, error) {
		if id != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			return app.Meta{}, nil, 0, errors.New("bad id")
		}
		return app.Meta{Version: 1, NonceB64u: "n1"}, io.NopCloser(bytes.NewReader([]byte("cipher"))), 6, nil
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if v := w.Header().Get("X-Gone-Version"); v != "1" {
		t.Fatalf("version header %s", v)
	}
	if n := w.Header().Get("X-Gone-Nonce"); n != "n1" {
		t.Fatalf("nonce header %s", n)
	}
	if !bytes.Equal(w.Body.Bytes(), []byte("cipher")) {
		t.Fatalf("body mismatch")
	}
}

func TestHandleConsumeNotFound(t *testing.T) {
	m := mockService{consumeFn: func(_ context.Context, _ string) (app.Meta, io.ReadCloser, int64, error) {
		return app.Meta{}, nil, 0, app.ErrNotFound
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", w.Code)
	}
}

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
