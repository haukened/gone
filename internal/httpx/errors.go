package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
)

// writeError writes a JSON error body with given status code.
func (h *Handler) writeError(ctx context.Context, w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
	if cid, ok := GetCorrelationID(ctx); ok {
		slog.Debug("wrote error response", "cid", cid, "status", code, "msg", msg)
	}
}

// mapServiceError maps domain/store/service errors to HTTP responses.
func (h *Handler) mapServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	cid, _ := GetCorrelationID(ctx)
	switch {
	case errors.Is(err, domain.ErrInvalidID):
		slog.Warn("service error", "cid", cid, "code", "invalid_id")
		h.writeError(ctx, w, http.StatusBadRequest, "invalid id")
	case errors.Is(err, app.ErrSizeExceeded):
		slog.Warn("service error", "cid", cid, "code", "size_exceeded")
		h.writeError(ctx, w, http.StatusRequestEntityTooLarge, "size exceeded")
	case errors.Is(err, app.ErrNotFound):
		slog.Info("service error", "cid", cid, "code", "not_found")
		h.writeError(ctx, w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrTTLInvalid):
		slog.Warn("service error", "cid", cid, "code", "ttl_invalid")
		h.writeError(ctx, w, http.StatusBadRequest, "ttl invalid")
	case errors.Is(err, os.ErrNotExist):
		slog.Info("service error", "cid", cid, "code", "not_found", "err_type", "os.ErrNotExist")
		h.writeError(ctx, w, http.StatusNotFound, "not found")
	default:
		// Internal / unexpected: do not log raw error string to avoid leaking IDs or paths.
		slog.Error("unhandled service error", "cid", cid, "code", "unhandled", "err_type", "unknown")
		h.writeError(ctx, w, http.StatusInternalServerError, "internal")
	}
}
