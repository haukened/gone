package httpx

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// handleConsumeSecret implements GET /api/secret/{id}.
func (h *Handler) handleConsumeSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	const prefix = "/api/secret/"
	if len(r.URL.Path) <= len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}
	id := r.URL.Path[len(prefix):]
	meta, rc, size, err := h.Service.Consume(r.Context(), id)
	if err != nil {
		h.mapServiceError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("X-Gone-Version", fmt.Sprintf("%d", meta.Version))
	w.Header().Set("X-Gone-Nonce", meta.NonceB64u)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.CopyN(w, rc, size)
}
