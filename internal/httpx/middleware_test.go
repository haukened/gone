package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestCorrelationIDMiddleware covers behavior of CorrelationIDMiddleware and GetCorrelationID.
func TestCorrelationIDMiddleware(t *testing.T) {
	tests := []struct {
		name                string
		requestHeaders      map[string]string
		expectStatus        int
		expectCallNext      bool
		expectReuseHeader   bool
		providedValue       string
		expectGeneratedUUID bool
		expectErrorContains string
	}{
		{
			name:                "generate when header missing",
			requestHeaders:      nil,
			expectStatus:        http.StatusOK,
			expectCallNext:      true,
			expectGeneratedUUID: true,
		},
		{
			name:              "reuse X-Correlation-ID header",
			requestHeaders:    map[string]string{CorrelationIDHeader: "123e4567-e89b-12d3-a456-426614174000"},
			expectStatus:      http.StatusOK,
			expectCallNext:    true,
			expectReuseHeader: true,
			providedValue:     "123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:                "reject invalid X-Correlation-ID header",
			requestHeaders:      map[string]string{CorrelationIDHeader: "abc123"},
			expectStatus:        http.StatusBadRequest,
			expectCallNext:      false,
			expectGeneratedUUID: true,
			expectErrorContains: "invalid correlation id",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var handlerCtxID string
			hitNext := false
			final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hitNext = true
				w.WriteHeader(http.StatusOK)
				id, ok := GetCorrelationID(r.Context())
				if !ok {
					t.Errorf("expected correlation ID in context")
				}
				handlerCtxID = id
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.requestHeaders {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			CorrelationIDMiddleware(final).ServeHTTP(rr, req)

			resp := rr.Result()
			gotHeader := resp.Header.Get(CorrelationIDHeader)
			if gotHeader == "" {
				t.Fatalf("expected response header %s to be set", CorrelationIDHeader)
			}

			if rr.Code != tt.expectStatus {
				t.Fatalf("expected status %d, got %d", tt.expectStatus, rr.Code)
			}

			if hitNext != tt.expectCallNext {
				t.Fatalf("expected next called=%v, got %v", tt.expectCallNext, hitNext)
			}

			if tt.expectErrorContains != "" && !strings.Contains(rr.Body.String(), tt.expectErrorContains) {
				t.Fatalf("expected response body %q to contain %q", rr.Body.String(), tt.expectErrorContains)
			}

			if handlerCtxID == "" {
				if tt.expectCallNext {
					t.Fatalf("expected context correlation ID to be set in handler")
				}
			}

			// Reuse case: value should match provided internal header.
			if tt.expectReuseHeader && gotHeader != tt.providedValue {
				t.Errorf("expected middleware to reuse provided value %q, got %q", tt.providedValue, gotHeader)
			}

			if tt.expectGeneratedUUID {
				if _, err := uuid.Parse(gotHeader); err != nil {
					t.Errorf("expected generated correlation ID to be a UUID, got %q: %v", gotHeader, err)
				}
			}

			// Handler context ID should always match header set by middleware.
			if tt.expectCallNext && handlerCtxID != gotHeader {
				t.Errorf("expected handler context ID %q to equal response header %q", handlerCtxID, gotHeader)
			}
		})
	}
}
