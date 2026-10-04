package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

func TestHandleAckDeleteRouted(t *testing.T) {
	token, err := domain.NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	called := false
	m := mockService{ackFn: func(_ context.Context, id, gotToken string) error {
		called = true
		if id != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			t.Fatalf("id = %q", id)
		}
		if gotToken != token.String() {
			t.Fatalf("ack token = %q, want %q", gotToken, token.String())
		}
		return nil
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.Header.Set(httpx.HeaderClaim, token.String())
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
	if !called {
		t.Fatalf("expected Ack to be called")
	}
}
