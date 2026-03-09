package httpx

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// correlationIDCtxKey is the unexported context key type to avoid collisions.
// We intentionally use a private struct{} key rather than a string to prevent
// accidental overwrites from other packages.
type correlationIDCtxKey struct{}

var cidKey = correlationIDCtxKey{}

// CorrelationIDHeader is the HTTP header used for inbound/outbound correlation IDs.
const CorrelationIDHeader = "X-Correlation-ID"

// CorrelationIDMiddleware injects a per-request correlation ID into the request
// context and response headers. If X-Correlation-ID is absent a new UUID v4 is
// generated. If a value is present it must parse as a UUID; invalid values are
// rejected with HTTP 400 and the chain is not continued. Downstream handlers can
// retrieve the value via GetCorrelationID.
func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get(CorrelationIDHeader)
		cid, ok := sanitizeCorrelationID(cid)
		if !ok {
			// Generate a fresh correlation ID for this error response so logs remain traceable.
			generated := uuid.New().String()
			ctx := context.WithValue(r.Context(), cidKey, generated)
			w.Header().Set(CorrelationIDHeader, generated)
			writeJSONError(ctx, w, http.StatusBadRequest, "invalid correlation id")
			return
		}
		// Store the CID in the request context for downstream handlers.
		ctx := context.WithValue(r.Context(), cidKey, cid)
		w.Header().Set(CorrelationIDHeader, cid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetCorrelationID extracts the correlation ID from the context. The second
// boolean return reports whether a value was present.
func GetCorrelationID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(cidKey).(string)
	return id, ok
}

// sanitizeCorrelationID validates and canonicalizes an inbound correlation ID.
//
// Parameters:
//   - cid: Header value to validate.
//
// Returns:
//   - string: Canonical UUID string (or generated UUID when input is empty).
//   - bool: True when input is accepted; false when input is invalid.
func sanitizeCorrelationID(cid string) (string, bool) {
	if cid == "" {
		return uuid.New().String(), true
	}
	uid, err := uuid.Parse(cid)
	if err != nil {
		return "", false
	}
	return uid.String(), true
}
