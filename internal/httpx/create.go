package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// requestMeta holds parsed and validated request metadata needed to create a secret.
type requestMeta struct {
	contentLength int64
	version       uint8
	nonce         string
	ttl           time.Duration
}

// parseAndValidateCreate extracts and validates headers and method/path invariants.
// It returns a populated requestMeta or an error describing the failure. Returned
// errors are mapped to HTTP status codes by classifyCreateError.
func checkMethodPath(r *http.Request) error {
	if r.Method != http.MethodPost {
		return errors.New("method not allowed")
	}
	if r.URL.Path != "/api/secret" {
		return errors.New("not found")
	}
	return nil
}

func (h *Handler) parseContentLength(r *http.Request) (int64, error) {
	clHeader := r.Header.Get("Content-Length")
	if clHeader == "" {
		return 0, errors.New("content length required")
	}
	cl, err := strconv.ParseInt(clHeader, 10, 64)
	if err != nil || cl <= 0 {
		return 0, errors.New("invalid content length")
	}
	if h.MaxBody > 0 && cl > h.MaxBody {
		return 0, errors.New("size exceeded")
	}
	return cl, nil
}

func parseSecretHeaders(r *http.Request) (uint8, string, time.Duration, error) {
	versionStr := r.Header.Get("X-Gone-Version")
	nonce := r.Header.Get("X-Gone-Nonce")
	ttlStr := r.Header.Get("X-Gone-TTL")
	if versionStr == "" || nonce == "" || ttlStr == "" {
		return 0, "", 0, errors.New("missing required headers")
	}
	v64, err := strconv.ParseUint(versionStr, 10, 8)
	if err != nil {
		return 0, "", 0, errors.New("invalid version")
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 0, "", 0, errors.New("invalid ttl")
	}
	return uint8(v64), nonce, ttl, nil
}

func (h *Handler) parseAndValidateCreate(r *http.Request) (*requestMeta, error) {
	if err := checkMethodPath(r); err != nil {
		return nil, err
	}
	cl, err := h.parseContentLength(r)
	if err != nil {
		return nil, err
	}
	ver, nonce, ttl, err := parseSecretHeaders(r)
	if err != nil {
		return nil, err
	}
	return &requestMeta{contentLength: cl, version: ver, nonce: nonce, ttl: ttl}, nil
}

// classifyCreateError maps validation error messages to HTTP status codes and
// user-facing error strings to keep handleCreateSecret concise.
func classifyCreateError(err error) (int, string) {
	if err == nil {
		return http.StatusInternalServerError, "internal error"
	}
	lookup := map[string]int{
		"method not allowed":       http.StatusMethodNotAllowed,
		"not found":                http.StatusNotFound,
		"content length required":  http.StatusLengthRequired,
		"invalid content length":   http.StatusBadRequest,
		"size exceeded":            http.StatusRequestEntityTooLarge,
		"missing required headers": http.StatusBadRequest,
		"invalid version":          http.StatusBadRequest,
		"invalid ttl":              http.StatusBadRequest,
	}
	msg := err.Error()
	if code, ok := lookup[msg]; ok {
		return code, msg
	}
	return http.StatusBadRequest, "bad request"
}

// handleCreateSecret implements POST /api/secret.
// handleCreateSecret implements POST /api/secret.
// It delegates validation to parseAndValidateCreate to reduce complexity.
func (h *Handler) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	meta, err := h.parseAndValidateCreate(r)
	if err != nil {
		code, msg := classifyCreateError(err)
		h.writeError(w, code, msg)
		return
	}
	body := http.MaxBytesReader(w, r.Body, meta.contentLength)
	defer body.Close()
	id, expires, svcErr := h.Service.CreateSecret(r.Context(), body, meta.contentLength, meta.version, meta.nonce, meta.ttl)
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
