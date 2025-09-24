package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleReady_NoReadiness ensures 200 when no readiness probe is configured.
func TestHandleReady_NoReadiness(t *testing.T) {
	h := &Handler{Readiness: nil}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	h.handleReady(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "ready" {
		t.Fatalf("expected body 'ready', got %q", body)
	}
}

func TestHandleReady_Ready(t *testing.T) {
	called := false
	h := &Handler{
		Readiness: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	h.handleReady(rr, req)

	if !called {
		t.Fatalf("expected readiness probe to be called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "ready" {
		t.Fatalf("expected body 'ready', got %q", body)
	}
}

// TestHandleReady_NotReady ensures 503 and an error body when readiness fails.
func TestHandleReady_NotReady(t *testing.T) {
	h := &Handler{
		Readiness: func(ctx context.Context) error {
			return errors.New("db unavailable")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	h.handleReady(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(strings.ToLower(body), "not ready") {
		t.Fatalf("expected body to contain 'not ready', got %q", body)
	}
}
