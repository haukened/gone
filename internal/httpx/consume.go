package httpx

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

// handleConsumeSecret implements GET /api/secret/{id}.
func (h *Handler) handleConsumeSecret(w http.ResponseWriter, r *http.Request) {
	// guard against unexpected methods, even though routing should prevent this.
	if r.Method != http.MethodGet {
		h.writeError(r.Context(), w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// guard against unexpected paths, even though routing should prevent this.
	const prefix = "/api/secret/"
	if len(r.URL.Path) <= len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	// create a correlation ID for logging if none exists yet
	// and use it for this request's logging context.
	cid, _ := GetCorrelationID(r.Context())
	clog := slog.With("domain", "secret", "cid", cid)
	clog.Info("consume", "action", "start")
	// extract ID from path
	id := r.URL.Path[len(prefix):]
	// attempt to consume the secret
	meta, rc, size, err := h.Service.Consume(r.Context(), id)
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Error("consume", "action", "error")
		return
	}
	defer rc.Close()
	// success: write headers and copy body
	w.Header().Set("X-Gone-Version", fmt.Sprintf("%d", meta.Version))
	w.Header().Set("X-Gone-Nonce", meta.NonceB64u)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	_, err = io.CopyN(w, rc, size)
	if err != nil {
		clog.Error("consume", "action", "error")
		return
	}
	clog.Info("consume", "action", "success")
}
