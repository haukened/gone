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
// context and response headers. If the incoming request already supplies
// X-Correlation-ID it is trusted (still not logged with any sensitive data). If
// absent a new UUID v4 is generated. Downstream handlers can retrieve the value
// via GetCorrelationID.
func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get(CorrelationIDHeader)
		if cid == "" {
			cid = uuid.New().String()
		}
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
