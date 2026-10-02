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

// writeJSONError writes a JSON error body with the provided HTTP status code and
// message and emits a debug log entry when a correlation ID exists in ctx.
//
// Parameters:
//   - ctx: Request-scoped context that may contain the correlation ID.
//   - w: HTTP response writer receiving headers/status/body.
//   - code: HTTP status code to return.
//   - msg: User-facing error message included in the JSON payload.
func writeJSONError(ctx context.Context, w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
}

// writeError writes a JSON error body with given status code.
func (h *Handler) writeError(ctx context.Context, w http.ResponseWriter, code int, msg string) {
	writeJSONError(ctx, w, code, msg)
}

// serviceErrorMapping describes how one service error class is reported.
type serviceErrorMapping struct {
	err    error
	status int
	msg    string
	code   string
	level  slog.Level
}

// serviceErrorTable lists known service errors in match order.
var serviceErrorTable = []serviceErrorMapping{
	{domain.ErrInvalidID, http.StatusBadRequest, "invalid id", "invalid_id", slog.LevelWarn},
	{domain.ErrInvalidClaim, http.StatusBadRequest, "invalid claim", "invalid_claim", slog.LevelWarn},
	{domain.ErrInvalidVersion, http.StatusBadRequest, "invalid version", "invalid_version", slog.LevelWarn},
	{domain.ErrInvalidNonce, http.StatusBadRequest, "invalid nonce", "invalid_nonce", slog.LevelWarn},
	{app.ErrSizeExceeded, http.StatusRequestEntityTooLarge, "size exceeded", "size_exceeded", slog.LevelWarn},
	{app.ErrNotFound, http.StatusNotFound, "not found", "not_found", slog.LevelInfo},
	{domain.ErrTTLInvalid, http.StatusBadRequest, "ttl invalid", "ttl_invalid", slog.LevelWarn},
	{os.ErrNotExist, http.StatusNotFound, "not found", "not_found", slog.LevelInfo},
}

// mapServiceError maps domain/store/service errors to HTTP responses.
// Unknown errors become 500 without logging the raw error string, to avoid
// leaking IDs or paths.
//
// Parameters:
//   - ctx: request context carrying the correlation ID.
//   - w: response writer.
//   - err: error returned by the service.
func (h *Handler) mapServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	cid, _ := GetCorrelationID(ctx)
	for _, m := range serviceErrorTable {
		if errors.Is(err, m.err) {
			slog.Log(ctx, m.level, "service error", "cid", cid, "code", m.code)
			h.writeError(ctx, w, m.status, m.msg)
			return
		}
	}
	slog.Error("unhandled service error", "cid", cid, "code", "unhandled", "err_type", "unknown")
	h.writeError(ctx, w, http.StatusInternalServerError, "internal")
}
