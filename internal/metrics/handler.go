package metrics

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// SnapshotProvider abstracts Manager for testing.
type SnapshotProvider interface {
	Snapshot(ctx context.Context) (map[string]int64, map[string]SummaryAgg, error)
}

// NewMux returns the metrics listener's handler. Every request must include
// Authorization: Bearer <token>; then GET / serves the JSON snapshot and
// GET /metrics serves the Prometheus text format. Other paths are 404 and
// other methods 405.
//
// Parameters:
//   - provider: persisted counters and summaries.
//   - token: the bearer token; empty rejects every request.
//   - sources: metrics computed at scrape time, for /metrics only.
//
// Returns:
//   - http.Handler: the routed, token-protected handler.
func NewMux(provider SnapshotProvider, token string, sources ...Source) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", jsonHandler(provider))
	mux.Handle("GET /metrics", PrometheusHandler(provider, sources...))
	return requireToken(token, mux)
}

// Handler returns an http.HandlerFunc that writes JSON metrics snapshot.
// Requests must include Authorization: Bearer <token>.
func Handler(provider SnapshotProvider, token string) http.HandlerFunc {
	return requireToken(token, jsonHandler(provider))
}

// requireToken rejects requests without the bearer token with 401.
//
// Parameters:
//   - token: the expected bearer token; empty rejects every request.
//   - next: handler for authorized requests.
//
// Returns:
//   - http.HandlerFunc: the guarded handler.
func requireToken(token string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r.Header.Get("Authorization"), token) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// jsonHandler writes the snapshot as JSON counters and summaries.
func jsonHandler(provider SnapshotProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		counters, summaries, err := provider.Snapshot(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Convert summaries (unexported fields) to JSON-friendly structure.
		outSummaries := make(map[string]map[string]int64, len(summaries))
		for k, v := range summaries {
			outSummaries[k] = map[string]int64{
				"count": v.Count,
				"sum":   v.Sum,
				"min":   v.Min,
				"max":   v.Max,
			}
		}
		resp := map[string]any{
			"counters":  counters,
			"summaries": outSummaries,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// authorized reports whether header is "Bearer " followed by token. Both sides
// are hashed before the constant-time comparison, so the comparison takes the
// same time whatever the length of the presented value.
//
// Parameters:
//   - header: the Authorization header value.
//   - token: the expected bearer token; empty never matches.
//
// Returns:
//   - bool: true only for an exact match.
func authorized(header, token string) bool {
	if token == "" {
		return false
	}
	got, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || got == "" {
		return false
	}
	want, have := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(have[:], want[:]) == 1
}
