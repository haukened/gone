package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
)

// writeJSONError writes a JSON error body with the provided HTTP status code and
// message.
//
// Parameters:
//   - _: unused request context kept for call-site symmetry.
//   - w: HTTP response writer receiving headers/status/body.
//   - code: HTTP status code to return.
//   - msg: User-facing error message included in the JSON payload.
func writeJSONError(_ context.Context, w http.ResponseWriter, code int, msg string) {
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

// errClientGone stands for a request whose context ended (the client hung up
// or timed out) while the service worked. It never reaches the service; see
// matchServiceError.
var errClientGone = errors.New("client gone")

// serviceErrorMapping describes how one service error class is reported. A
// message of "" means a status line only, with no body.
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
	{domain.ErrInvalidManage, http.StatusBadRequest, "invalid manage token", "invalid_manage", slog.LevelWarn},
	{domain.ErrInvalidFill, http.StatusBadRequest, "invalid fill token", "invalid_fill", slog.LevelWarn},
	{app.ErrRequestsDisabled, http.StatusNotFound, "not found", "requests_disabled", slog.LevelInfo},
	{domain.ErrInvalidVersion, http.StatusBadRequest, "invalid version", "invalid_version", slog.LevelWarn},
	{domain.ErrInvalidNonce, http.StatusBadRequest, "invalid nonce", "invalid_nonce", slog.LevelWarn},
	{app.ErrSizeExceeded, http.StatusRequestEntityTooLarge, "size exceeded", "size_exceeded", slog.LevelWarn},
	{app.ErrNotFound, http.StatusNotFound, "not found", "not_found", slog.LevelInfo},
	{domain.ErrTTLInvalid, http.StatusBadRequest, "ttl invalid", "ttl_invalid", slog.LevelWarn},
	{os.ErrNotExist, http.StatusNotFound, "not found", "not_found", slog.LevelInfo},
	{app.ErrBusy, http.StatusServiceUnavailable, "busy", "busy", slog.LevelWarn},
	{io.ErrUnexpectedEOF, http.StatusBadRequest, "short body", "short_body", slog.LevelWarn},
	{io.EOF, http.StatusBadRequest, "short body", "short_body", slog.LevelWarn},
	{errClientGone, statusClientClosed, "", "client_gone", slog.LevelDebug},
}

// statusClientClosed is the nginx-style status recorded for a request whose
// client went away. Nobody reads it, but writing it stops the router's
// fallback from adding a 404 body.
const statusClientClosed = 499

// busyRetryAfter is the Retry-After value, in seconds, sent with 503 busy.
const busyRetryAfter = "1"

// matchServiceError finds the table entry for err. When the request context
// has ended, whatever the service returned (a context error, or a read error
// from the dropped connection) is the client's doing and matches errClientGone.
//
// Parameters:
//   - ctx: request context.
//   - err: error returned by the service.
//
// Returns the mapping and whether one matched.
func matchServiceError(ctx context.Context, err error) (serviceErrorMapping, bool) {
	if ctx.Err() != nil {
		err = errClientGone
	}
	for _, m := range serviceErrorTable {
		if errors.Is(err, m.err) {
			return m, true
		}
	}
	return serviceErrorMapping{}, false
}

// mapServiceError maps domain/store/service errors to HTTP responses.
// Unknown errors become 500 without logging the raw error string, to avoid
// leaking IDs or paths. A body shorter than its Content-Length is a 400, and
// a request whose client has gone gets a bare 499 and no error log.
//
// Parameters:
//   - ctx: request context carrying the correlation ID.
//   - w: response writer.
//   - err: error returned by the service.
func (h *Handler) mapServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	cid, _ := GetCorrelationID(ctx)
	m, ok := matchServiceError(ctx, err)
	if !ok {
		slog.Error("unhandled service error", "cid", cid, "code", "unhandled", "err_type", "unknown")
		h.writeError(ctx, w, http.StatusInternalServerError, "internal")
		return
	}
	slog.Log(ctx, m.level, "service error", "cid", cid, "code", m.code)
	if m.msg == "" {
		w.WriteHeader(m.status)
		return
	}
	if m.status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", busyRetryAfter)
	}
	h.writeError(ctx, w, m.status, m.msg)
}
