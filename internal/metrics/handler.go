package metrics

import (
	"context"
	"encoding/json"
	"net/http"
)

// SnapshotProvider abstracts Manager for testing.
type SnapshotProvider interface {
	Snapshot(ctx context.Context) (map[string]int64, map[string]summaryAgg, error)
}

// Handler returns an http.HandlerFunc that writes JSON metrics snapshot.
// If token is non-empty, requests must include Authorization: Bearer <token>.
func Handler(provider SnapshotProvider, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			hdr := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(hdr) <= len(prefix) || hdr[:len(prefix)] != prefix || hdr[len(prefix):] != token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		counters, summaries, err := provider.Snapshot(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Convert summaries (unexported fields) to JSON-friendly structure.
		outSummaries := make(map[string]map[string]int64, len(summaries))
		for k, v := range summaries {
			outSummaries[k] = map[string]int64{
				"count": v.count,
				"sum":   v.sum,
				"min":   v.min,
				"max":   v.max,
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
