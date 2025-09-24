package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TestCorrelationIdMiddleware_GeneratesWhenAbsent verifies a new correlation ID is generated
// when neither the custom lookup header ("correlationID") nor the response header is provided.
func TestCorrelationIdMiddleware_GeneratesWhenAbsent(t *testing.T) {
	t.Parallel()

	var (
		ctxValue        any
		getID           CorrelationID
		getIDOK         bool
		responseHeader  string
		nextCalledCount int
	)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalledCount++
		ctxValue = r.Context().Value(CorrelationIdKey)
		getID, getIDOK = GetCorrelationID(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	CorrelationIdMiddleware(next).ServeHTTP(rr, req)

	if nextCalledCount != 1 {
		t.Fatalf("expected next handler to be called once, got %d", nextCalledCount)
	}

	responseHeader = rr.Header().Get(CorrelationIDHeader)
	if responseHeader == "" {
		t.Fatalf("expected %s header to be set", CorrelationIDHeader)
	}

	if _, err := uuid.Parse(responseHeader); err != nil {
		t.Fatalf("generated correlation ID not a valid UUID: %v", err)
	}

	// Middleware stores a plain string; GetCorrelationID expects CorrelationID type, so it will currently fail.
	if getIDOK {
		t.Fatalf("expected GetCorrelationID to fail due to type mismatch, but succeeded with %q", getID)
	}

	s, ok := ctxValue.(string)
	if !ok {
		t.Fatalf("expected context value to be a string, got %T", ctxValue)
	}
	if s != responseHeader {
		t.Fatalf("context value %q does not match response header %q", s, responseHeader)
	}
}

// TestCorrelationIdMiddleware_PreservesExistingCustomHeader ensures that if the request supplies
// the header key actually looked up ("correlationID"), it is propagated.
func TestCorrelationIdMiddleware_PreservesExistingCustomHeader(t *testing.T) {
	t.Parallel()

	existing := uuid.New().String()

	var ctxValue any
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxValue = r.Context().Value(CorrelationIdKey)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// NOTE: The middleware looks for CorrelationIdKey ("correlationID"), not CorrelationIDHeader.
	req.Header.Set(CorrelationIdKey, existing)

	rr := httptest.NewRecorder()
	CorrelationIdMiddleware(next).ServeHTTP(rr, req)

	respID := rr.Header().Get(CorrelationIDHeader)
	if respID != existing {
		t.Fatalf("expected response header %s to be %q, got %q", CorrelationIDHeader, existing, respID)
	}

	if ctxValue.(string) != existing { // nolint:forcetypeassert - we expect string per current implementation
		t.Fatalf("expected context correlation ID %q, got %v", existing, ctxValue)
	}
}

// TestCorrelationIdMiddleware_IgnoresStandardHeader documents the current behavior where providing
// the conventional X-Correlation-ID header does NOT get picked up (a new ID is generated instead).
func TestCorrelationIdMiddleware_IgnoresStandardHeader(t *testing.T) {
	t.Parallel()

	provided := uuid.New().String()

	var ctxValue any
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxValue = r.Context().Value(CorrelationIdKey)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(CorrelationIDHeader, provided) // This will be ignored by current middleware logic.

	rr := httptest.NewRecorder()
	CorrelationIdMiddleware(next).ServeHTTP(rr, req)

	respID := rr.Header().Get(CorrelationIDHeader)
	if respID == "" {
		t.Fatalf("expected middleware to set %s header", CorrelationIDHeader)
	}
	if respID == provided {
		t.Fatalf("expected generated ID to differ from provided header (%s), but they match", provided)
	}
	if ctxValue.(string) != respID { // nolint:forcetypeassert
		t.Fatalf("context value %v does not match response header %s", ctxValue, respID)
	}
}
