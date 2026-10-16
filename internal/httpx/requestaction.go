package httpx

import (
	"errors"
	"net/http"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
)

// handleRequestOpen implements GET /api/request/{id}: 200 while the request
// can be answered, the uniform 404 otherwise. It changes nothing.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleRequestOpen(w http.ResponseWriter, r *http.Request, id string) {
	expires, err := h.Requests.RequestOpen(r.Context(), id, singleValue(r, HeaderFill))
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		State     string    `json:"state"`
		ExpiresAt time.Time `json:"expires_at"`
	}{"open", expires.UTC()})
}

// handleFillRequest implements PUT /api/request/{id}/reply. Size and protocol
// headers follow POST /api/secret, except the version must be 3 and there is
// no TTL.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleFillRequest(w http.ResponseWriter, r *http.Request, id string) {
	clog := requestLog(r)
	clog.Info("request fill", "action", "start")
	cl, version, nonce, err := h.parseFillHeaders(r)
	if err != nil {
		code, msg := classifyCreateError(err)
		h.writeError(r.Context(), w, code, msg)
		clog.Info("request fill", "action", "error", "kind", "validation")
		return
	}
	body := http.MaxBytesReader(w, r.Body, cl)
	defer func() { _ = body.Close() }()
	expires, err := h.Requests.Fill(r.Context(), id, singleValue(r, HeaderFill), body, cl, version, nonce)
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Info("request fill", "action", "error", "kind", "service")
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		ExpiresAt time.Time `json:"expires_at"`
	}{expires.UTC()})
	clog.Info("request fill", "action", "success")
}

// parseFillHeaders validates the size and protocol headers of a reply.
//
// Parameters:
//   - r: incoming request.
//
// Returns the content length, version and nonce, or an error whose message
// is a key of classifyCreateError's lookup table.
func (h *Handler) parseFillHeaders(r *http.Request) (int64, uint8, string, error) {
	cl, err := h.parseContentLength(r)
	if err != nil {
		return 0, 0, "", err
	}
	versionStr, okV := singleHeader(r, "X-Gone-Version")
	nonce, okN := singleHeader(r, "X-Gone-Nonce")
	if !okV || !okN {
		return 0, 0, "", errors.New("missing required headers")
	}
	version, err := domain.ParseReplyVersion(versionStr)
	if err != nil {
		return 0, 0, "", err
	}
	if err = domain.ValidateReplyProtocol(version, nonce); err != nil {
		return 0, 0, "", err
	}
	return cl, version, nonce, nil
}

// handleRequestStatus implements GET /api/request/{id}/status for the
// requester: waiting or ready, or the uniform 404. It changes nothing, so
// polling costs no writes.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleRequestStatus(w http.ResponseWriter, r *http.Request, id string) {
	st, err := h.Requests.RequestStatus(r.Context(), id, manageToken(r))
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		return
	}
	state := "waiting"
	if st.Ready {
		state = "ready"
	}
	writeJSON(w, http.StatusOK, statusResponse{State: state, CreatedAt: st.CreatedAt.UTC(), ExpiresAt: st.ExpiresAt.UTC()})
}

// handleClaimReply implements GET /api/request/{id}/reply: the requester
// claims (or re-fetches) the reply, with the same response headers as a
// secret claim.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleClaimReply(w http.ResponseWriter, r *http.Request, id string) {
	clog := requestLog(r)
	token, err := claimHeader(r)
	var res app.ClaimResult
	if err == nil {
		res, err = h.Requests.ClaimReply(r.Context(), id, manageToken(r), token)
	}
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Info("request claim", "action", "error")
		return
	}
	h.writeClaim(w, res, clog)
}

// handleAckReply implements DELETE /api/request/{id}/reply: the requester
// confirms the reply was opened and it is deleted.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleAckReply(w http.ResponseWriter, r *http.Request, id string) {
	token, err := claimHeader(r)
	if err == nil {
		err = h.Requests.AckReply(r.Context(), id, token)
	}
	h.finishNoContent(w, r, err, "request ack")
}

// handleCancelRequest implements POST /api/request/{id}/revoke.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleCancelRequest(w http.ResponseWriter, r *http.Request, id string) {
	h.finishNoContent(w, r, h.Requests.CancelRequest(r.Context(), id, manageToken(r)), "request cancel")
}
