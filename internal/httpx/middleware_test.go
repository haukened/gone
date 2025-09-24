package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TestCorrelationIDMiddleware covers behavior of CorrelationIDMiddleware and GetCorrelationID.
func TestCorrelationIDMiddleware(t *testing.T) {
	tests := []struct {
		name                string
		requestHeaders      map[string]string
		expectReuseHeader   bool
		providedValue       string
		expectGeneratedUUID bool
	}{
		{
			name:                "generate when header missing",
			requestHeaders:      nil,
			expectGeneratedUUID: true,
		},
		{
			name:              "reuse X-Correlation-ID header",
			requestHeaders:    map[string]string{CorrelationIDHeader: "abc123"},
			expectReuseHeader: true,
			providedValue:     "abc123",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var handlerCtxID string
			final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			if handlerCtxID == "" {
				t.Fatalf("expected context correlation ID to be set in handler")
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
			if handlerCtxID != gotHeader {
				t.Errorf("expected handler context ID %q to equal response header %q", handlerCtxID, gotHeader)
			}
		})
	}
}
