package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
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

// singleHeader returns the sole value of header name. ok is false when the
// header is absent, repeated, or empty.
//
// Parameters:
//   - r: incoming request.
//   - name: canonical header name.
//
// Returns the value and whether exactly one non-empty value was present.
func singleHeader(r *http.Request, name string) (string, bool) {
	vals := r.Header.Values(name)
	if len(vals) != 1 || vals[0] == "" {
		return "", false
	}
	return vals[0], true
}

// parseSecretHeaders validates the protocol headers of a create request
// (docs/protocol.md §8.1). Each header must appear exactly once.
//
// Parameters:
//   - r: incoming request.
//
// Returns the version, nonce and TTL, or an error whose message is a key of
// classifyCreateError's lookup table.
func parseSecretHeaders(r *http.Request) (uint8, string, time.Duration, error) {
	versionStr, okV := singleHeader(r, "X-Gone-Version")
	nonce, okN := singleHeader(r, "X-Gone-Nonce")
	ttlStr, okT := singleHeader(r, "X-Gone-TTL")
	if !okV || !okN || !okT {
		return 0, "", 0, errors.New("missing required headers")
	}
	version, err := domain.ParseVersion(versionStr)
	if err != nil {
		return 0, "", 0, err
	}
	if err := domain.ValidateProtocol(version, nonce); err != nil {
		return 0, "", 0, err
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 0, "", 0, errors.New("invalid ttl")
	}
	return version, nonce, ttl, nil
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
		"invalid nonce":            http.StatusBadRequest,
		"invalid ttl":              http.StatusBadRequest,
	}
	msg := err.Error()
	if code, ok := lookup[msg]; ok {
		return code, msg
	}
	return http.StatusBadRequest, "bad request"
}

// handleCreateSecret implements POST /api/secret. It delegates validation to
// parseAndValidateCreate and responds 201 with the secret ID, expiry, and the
// sender's manage token. The manage token is never logged.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	cid, _ := GetCorrelationID(r.Context())
	clog := slog.With("domain", "secret", "cid", cid)
	clog.Info("create", "action", "start")
	meta, err := h.parseAndValidateCreate(r)
	if err != nil {
		code, msg := classifyCreateError(err)
		h.writeError(r.Context(), w, code, msg)
		clog.Error("create", "action", "error", "kind", "validation")
		return
	}
	body := http.MaxBytesReader(w, r.Body, meta.contentLength)
	defer func() { _ = body.Close() }()
	created, svcErr := h.Service.CreateSecret(r.Context(), body, meta.contentLength, meta.version, meta.nonce, meta.ttl)
	if svcErr != nil {
		h.mapServiceError(r.Context(), w, svcErr)
		clog.Error("create", "action", "error", "kind", "service")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		ID          string    `json:"id"`
		ExpiresAt   time.Time `json:"expires_at"`
		ManageToken string    `json:"manage_token"`
	}{ID: created.ID.String(), ExpiresAt: created.ExpiresAt, ManageToken: created.ManageToken.String()})
	clog.Info("create", "action", "success", "ttl_secs", int(meta.ttl.Seconds()))
}
