package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
)

// writeError writes a JSON error body with given status code.
func (h *Handler) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
}

// mapServiceError maps domain/store/service errors to HTTP responses.
func (h *Handler) mapServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidID):
		h.writeError(w, http.StatusBadRequest, "invalid id")
	case errors.Is(err, app.ErrSizeExceeded):
		h.writeError(w, http.StatusRequestEntityTooLarge, "size exceeded")
	case errors.Is(err, app.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrTTLInvalid):
		h.writeError(w, http.StatusBadRequest, "ttl invalid")
	default:
		h.writeError(w, http.StatusInternalServerError, "internal")
	}
}
