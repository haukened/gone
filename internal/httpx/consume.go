package httpx

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Claim protocol headers.
const (
	// HeaderClaim carries the claim token: issued in GET responses, and
	// presented by clients on GET retries and on DELETE acknowledgements.
	HeaderClaim = "X-Gone-Claim"
	// HeaderClaimExpires carries the RFC 3339 UTC lease deadline for the claim.
	HeaderClaimExpires = "X-Gone-Claim-Expires"
)

// handleConsumeSecret dispatches /api/secret/{id}[/{action}] by action and
// method:
//   - GET /api/secret/{id} claims (or re-fetches with an existing claim) the ciphertext.
//   - DELETE /api/secret/{id} acknowledges receipt and permanently deletes the secret.
//   - GET /api/secret/{id}/status reports a pending secret to its sender.
//   - POST /api/secret/{id}/revoke deletes a pending secret for its sender.
//
// Any other action segment is 404; a known action with the wrong method is 405.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleConsumeSecret(w http.ResponseWriter, r *http.Request) {
	route, ok := consumeSecretRouteFromPath(r.URL.Path)
	if !ok {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	h.dispatchConsumeSecretRoute(w, r, route)
}

// consumeSecretRoute identifies the secret and optional consume action.
type consumeSecretRoute struct {
	id     string
	action string
}

// consumeSecretRouteFromPath extracts a consume-secret route from path.
//
// Parameters:
//   - path: request URL path.
//
// Returns the route and true when the path has a non-empty secret suffix.
func consumeSecretRouteFromPath(path string) (consumeSecretRoute, bool) {
	rest, ok := strings.CutPrefix(path, "/api/secret/")
	if !ok || rest == "" {
		return consumeSecretRoute{}, false
	}
	id, action, _ := strings.Cut(rest, "/")
	return consumeSecretRoute{id: id, action: action}, true
}

// dispatchConsumeSecretRoute routes a parsed consume-secret request.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - route: parsed secret route.
func (h *Handler) dispatchConsumeSecretRoute(w http.ResponseWriter, r *http.Request, route consumeSecretRoute) {
	switch route.action {
	case "":
		h.dispatchSecret(w, r, route.id)
	case "status":
		if h.allowMethod(w, r, http.MethodGet) {
			h.handleStatusSecret(w, r, route.id)
		}
	case "revoke":
		if h.allowMethod(w, r, http.MethodPost) {
			h.handleRevokeSecret(w, r, route.id)
		}
	default:
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
	}
}

// dispatchSecret routes /api/secret/{id} by method: GET claims, DELETE acks.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: secret ID extracted from the path.
func (h *Handler) dispatchSecret(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		h.handleClaimSecret(w, r, id)
	case http.MethodDelete:
		h.handleAckSecret(w, r, id)
	default:
		w.Header().Set("Allow", "GET, DELETE")
		h.writeError(r.Context(), w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// allowMethod reports whether r uses method, writing 405 with an Allow header
// when it does not.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - method: the only permitted HTTP method.
//
// Returns true if the request may proceed.
func (h *Handler) allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	h.writeError(r.Context(), w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// handleClaimSecret implements GET /api/secret/{id}. Without an X-Gone-Claim
// request header it makes a fresh claim; with one it retries an existing
// claim. The secret is not deleted until the client sends DELETE with the
// issued token. The claim token is never logged.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: secret ID extracted from the path.
func (h *Handler) handleClaimSecret(w http.ResponseWriter, r *http.Request, id string) {
	cid, _ := GetCorrelationID(r.Context())
	token := r.Header.Get(HeaderClaim)
	clog := slog.With("domain", "secret", "cid", cid, "retry", token != "")
	clog.Info("claim", "action", "start")
	res, err := h.Service.Claim(r.Context(), id, token)
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Error("claim", "action", "error")
		return
	}
	defer func() { _ = res.Body.Close() }()
	hdr := w.Header()
	hdr.Set("X-Gone-Version", fmt.Sprintf("%d", res.Meta.Version))
	hdr.Set("X-Gone-Nonce", res.Meta.NonceB64u)
	hdr.Set(HeaderClaim, res.Token.String())
	hdr.Set(HeaderClaimExpires, res.ClaimedUntil.UTC().Format(time.RFC3339))
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("Content-Type", "application/octet-stream")
	hdr.Set("Content-Length", strconv.FormatInt(res.Size, 10))
	w.WriteHeader(http.StatusOK)
	if _, err = io.CopyN(w, res.Body, res.Size); err != nil {
		clog.Error("claim", "action", "error")
		return
	}
	clog.Info("claim", "action", "success")
}

// handleAckSecret implements DELETE /api/secret/{id}. The X-Gone-Claim header
// must carry the token issued by the claiming GET. On success the secret is
// permanently deleted and 204 No Content is returned.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: secret ID extracted from the path.
func (h *Handler) handleAckSecret(w http.ResponseWriter, r *http.Request, id string) {
	cid, _ := GetCorrelationID(r.Context())
	clog := slog.With("domain", "secret", "cid", cid)
	clog.Info("ack", "action", "start")
	if err := h.Service.Ack(r.Context(), id, r.Header.Get(HeaderClaim)); err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Error("ack", "action", "error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
	clog.Info("ack", "action", "success")
}
