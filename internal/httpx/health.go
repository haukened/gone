package httpx

import "net/http"

// handleHealth returns liveness.
func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleReady returns readiness; if probe unavailable or failing => 503.
func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if h.Readiness != nil {
		if err := h.Readiness(r.Context()); err != nil {
			h.writeError(w, http.StatusServiceUnavailable, "not ready")
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
