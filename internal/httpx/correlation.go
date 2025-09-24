package httpx

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type CorrelationID string

const CorrelationIDHeader = "X-Correlation-ID"
const CorrelationIdKey = "correlationID"

// CorrelationIdMiddleware ensures each request has a correlation ID, generating one if absent.
// The ID is added to the request context and response headers for tracing.

func CorrelationIdMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get(CorrelationIdKey)
		if cid == "" {
			cid = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), CorrelationIdKey, cid)
		r = r.WithContext(ctx)

		w.Header().Set(CorrelationIDHeader, cid)
		next.ServeHTTP(w, r)
	})
}

func GetCorrelationID(ctx context.Context) (CorrelationID, bool) {
	id, ok := ctx.Value(CorrelationIdKey).(CorrelationID)
	return id, ok
}
