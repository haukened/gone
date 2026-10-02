package httpx

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
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

// handleConsumeSecret dispatches /api/secret/{id} by method:
//   - GET claims (or re-fetches with an existing claim) the ciphertext.
//   - DELETE acknowledges receipt and permanently deletes the secret.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleConsumeSecret(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/secret/"
	if len(r.URL.Path) <= len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	id := r.URL.Path[len(prefix):]
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
	defer res.Body.Close()
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
