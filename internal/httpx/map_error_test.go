package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
)

func TestMapServiceError(t *testing.T) {
	h := &Handler{}
	cases := []struct {
		name string
		err  error
		code int
		body string
	}{
		{"invalid id", domain.ErrInvalidID, http.StatusBadRequest, "invalid id"},
		{"size exceeded", app.ErrSizeExceeded, http.StatusRequestEntityTooLarge, "size exceeded"},
		{"not found", app.ErrNotFound, http.StatusNotFound, "not found"},
		{"ttl invalid", domain.ErrTTLInvalid, http.StatusBadRequest, "ttl invalid"},
		{"invalid version", domain.ErrInvalidVersion, http.StatusBadRequest, "invalid version"},
		{"invalid nonce", domain.ErrInvalidNonce, http.StatusBadRequest, "invalid nonce"},
		{"os not exist", os.ErrNotExist, http.StatusNotFound, "not found"},
		{"busy", fmt.Errorf("insert: %w", app.ErrBusy), http.StatusServiceUnavailable, "busy"},
		{"short body", fmt.Errorf("save: %w", io.ErrUnexpectedEOF), http.StatusBadRequest, "short body"},
		{"empty body", io.EOF, http.StatusBadRequest, "short body"},
		{"internal default", errors.New("boom"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.mapServiceError(context.Background(), rr, tc.err)
			if rr.Code != tc.code {
				t.Fatalf("expected code %d got %d body=%s", tc.code, rr.Code, rr.Body.String())
			}
			if rr.Body.String() == "" || !containsJSONError(rr.Body.String(), tc.body) {
				t.Fatalf("expected body to contain %q got %s", tc.body, rr.Body.String())
			}
			wantRetry := ""
			if tc.code == http.StatusServiceUnavailable {
				wantRetry = busyRetryAfter
			}
			if got := rr.Header().Get("Retry-After"); got != wantRetry {
				t.Fatalf("Retry-After = %q, want %q", got, wantRetry)
			}
		})
	}
}

// containsJSONError performs a simple substring check for the error message in the JSON payload.
func containsJSONError(s, substr string) bool {
	return len(s) > 0 && // naive substring check is enough here
		// typical format: {"error":"<message>"}\n
		// no need for full JSON parsing
		// ensure we look for quoted value
		// substring presence validated
		(len(substr) > 0 && (indexOf(s, substr) != -1))
}

func indexOf(haystack, needle string) int {
	// small helper to avoid importing strings just for one call
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestMapServiceError_ContextEnded checks any error under an ended request
// context maps to a bare 499, while a live context with a context error does
// not (it is a server-side failure).
func TestMapServiceError_ContextEnded(t *testing.T) {
	h := &Handler{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rr := httptest.NewRecorder()
	h.mapServiceError(ctx, rr, errors.New("read: connection reset"))
	if rr.Code != statusClientClosed || rr.Body.Len() != 0 {
		t.Fatalf("ended ctx: got %d %q", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.mapServiceError(context.Background(), rr, context.DeadlineExceeded)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("live ctx with deadline error: got %d", rr.Code)
	}
}
