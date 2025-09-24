package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleConsumeSecret_EarlyFailures covers paths where the handler returns
// before invoking the Service (so we don't need a mock Service).
func TestHandleConsumeSecret_EarlyFailures(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{
			name:       "method not allowed",
			method:     http.MethodPost,
			target:     "/api/secret/abc123",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "not found - missing id (exact prefix length)",
			method:     http.MethodGet,
			target:     "/api/secret/", // len <= prefix => 404
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "not found - different prefix",
			method:     http.MethodGet,
			target:     "/api/secrets/abc123",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "not found - no trailing slash",
			method:     http.MethodGet,
			target:     "/api/secret", // shorter than prefix
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			rr := httptest.NewRecorder()

			h := &Handler{} // Service not needed for these early-return paths
			h.handleConsumeSecret(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, rr.Code)
			}
		})
	}
}
