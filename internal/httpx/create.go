package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// handleCreateSecret implements POST /api/secret.
func (h *Handler) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.URL.Path != "/api/secret" { // disallow trailing slash variants
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}
	clHeader := r.Header.Get("Content-Length")
	if clHeader == "" {
		h.writeError(w, http.StatusLengthRequired, "content length required")
		return
	}
	cl, err := strconv.ParseInt(clHeader, 10, 64)
	if err != nil || cl <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid content length")
		return
	}
	if h.MaxBody > 0 && cl > h.MaxBody {
		h.writeError(w, http.StatusRequestEntityTooLarge, "size exceeded")
		return
	}
	versionStr := r.Header.Get("X-Gone-Version")
	nonce := r.Header.Get("X-Gone-Nonce")
	ttlStr := r.Header.Get("X-Gone-TTL")
	if versionStr == "" || nonce == "" || ttlStr == "" {
		h.writeError(w, http.StatusBadRequest, "missing required headers")
		return
	}
	v64, err := strconv.ParseUint(versionStr, 10, 8)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid version")
		return
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid ttl")
		return
	}
	body := http.MaxBytesReader(w, r.Body, cl)
	defer body.Close()
	id, expires, svcErr := h.Service.CreateSecret(r.Context(), body, cl, uint8(v64), nonce, ttl)
	if svcErr != nil {
		h.mapServiceError(w, svcErr)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		ID        string    `json:"id"`
		ExpiresAt time.Time `json:"expires_at"`
	}{ID: id.String(), ExpiresAt: expires})
}
