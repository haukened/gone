package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
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
		{"os not exist", os.ErrNotExist, http.StatusNotFound, "not found"},
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
