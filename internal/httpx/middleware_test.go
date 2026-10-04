package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type correlationMiddlewareCase struct {
	name                string
	requestHeaders      map[string]string
	expectStatus        int
	expectCallNext      bool
	expectReuseHeader   bool
	providedValue       string
	expectGeneratedUUID bool
	expectErrorContains string
}

// TestCorrelationIDMiddleware covers behavior of CorrelationIDMiddleware and GetCorrelationID.
func TestCorrelationIDMiddleware(t *testing.T) {
	tests := []correlationMiddlewareCase{
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
		t.Run(tt.name, func(t *testing.T) {
			runCorrelationMiddlewareCase(t, tt)
		})
	}
}

// runCorrelationMiddlewareCase executes one middleware case and checks the result.
// It takes t for failures and tc as the scenario under test.
func runCorrelationMiddlewareCase(t *testing.T, tc correlationMiddlewareCase) {
	t.Helper()
	handlerCtxID := ""
	hitNext := false
	final := correlationFinalHandler(&hitNext, &handlerCtxID)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range tc.requestHeaders {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	CorrelationIDMiddleware(final).ServeHTTP(rr, req)
	gotHeader := rr.Result().Header.Get(CorrelationIDHeader)
	assertCorrelationBasics(t, tc, rr, gotHeader, hitNext)
	assertCorrelationIDs(t, tc, gotHeader, handlerCtxID)
}

// correlationFinalHandler records whether the wrapped handler ran and which ID it saw.
// It takes hitNext and handlerCtxID pointers to fill, and returns the final handler.
func correlationFinalHandler(hitNext *bool, handlerCtxID *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hitNext = true
		w.WriteHeader(http.StatusOK)
		id, ok := GetCorrelationID(r.Context())
		if ok {
			*handlerCtxID = id
		}
	})
}

// assertCorrelationBasics verifies response status, header presence, and error body text.
// It takes t for failures, tc as expectations, rr as the response, gotHeader, and hitNext.
func assertCorrelationBasics(t *testing.T, tc correlationMiddlewareCase, rr *httptest.ResponseRecorder, gotHeader string, hitNext bool) {
	t.Helper()
	if gotHeader == "" {
		t.Fatalf("expected response header %s to be set", CorrelationIDHeader)
	}
	if rr.Code != tc.expectStatus {
		t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
	}
	if hitNext != tc.expectCallNext {
		t.Fatalf("expected next called=%v, got %v", tc.expectCallNext, hitNext)
	}
	if tc.expectErrorContains != "" && !strings.Contains(rr.Body.String(), tc.expectErrorContains) {
		t.Fatalf("expected response body %q to contain %q", rr.Body.String(), tc.expectErrorContains)
	}
}

// assertCorrelationIDs verifies reused, generated, and context correlation IDs.
// It takes t for failures, tc as expectations, gotHeader, and handlerCtxID.
func assertCorrelationIDs(t *testing.T, tc correlationMiddlewareCase, gotHeader, handlerCtxID string) {
	t.Helper()
	if tc.expectCallNext && handlerCtxID == "" {
		t.Fatal("expected context correlation ID to be set in handler")
	}
	assertCorrelationHeaderSource(t, tc, gotHeader)
	if tc.expectCallNext && handlerCtxID != gotHeader {
		t.Errorf("expected handler context ID %q to equal response header %q", handlerCtxID, gotHeader)
	}
}

// assertCorrelationHeaderSource verifies whether the response ID was reused or generated.
// It takes t for failures, tc as expectations, and gotHeader as the observed response header.
func assertCorrelationHeaderSource(t *testing.T, tc correlationMiddlewareCase, gotHeader string) {
	t.Helper()
	if tc.expectReuseHeader && gotHeader != tc.providedValue {
		t.Errorf("expected middleware to reuse provided value %q, got %q", tc.providedValue, gotHeader)
	}
	if tc.expectGeneratedUUID {
		if _, err := uuid.Parse(gotHeader); err != nil {
			t.Errorf("expected generated correlation ID to be a UUID, got %q: %v", gotHeader, err)
		}
	}
}
