package httpx

import "net/http"

// allowProbeMethod reports whether r may use a probe route, writing 405 with
// an Allow header when it may not. Probes answer GET and HEAD only.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//
// Returns true if the request may proceed.
func (h *Handler) allowProbeMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	h.writeError(r.Context(), w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// handleHealth returns liveness: 200 "ok" to GET and HEAD, 405 otherwise.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !h.allowProbeMethod(w, r) {
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleReady returns readiness; if probe unavailable or failing => 503.
// Like liveness it answers GET and HEAD only.
func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if !h.allowProbeMethod(w, r) {
		return
	}
	if h.Readiness != nil {
		if err := h.Readiness(r.Context()); err != nil {
			h.writeError(r.Context(), w, http.StatusServiceUnavailable, "not ready")
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
