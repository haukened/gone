package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
)

// HeaderFill carries a request's fill token on the open check and the reply.
// It must appear exactly once; the token is never logged.
const HeaderFill = "X-Gone-Fill"

// RequestPort is the application interface for secret requests
// (docs/protocol.md §8.6).
type RequestPort interface {
	CreateRequest(ctx context.Context, ttl time.Duration) (app.CreatedRequest, error)
	RequestOpen(ctx context.Context, idStr, fill string) (time.Time, error)
	Fill(ctx context.Context, idStr, fill string, ct io.Reader, size int64, version uint8, nonce string) (time.Time, error)
	RequestStatus(ctx context.Context, idStr, token string) (app.RequestStatus, error)
	ClaimReply(ctx context.Context, idStr, manage, claim string) (app.ClaimResult, error)
	AckReply(ctx context.Context, idStr, claim string) error
	CancelRequest(ctx context.Context, idStr, token string) error
}

// requestRoute identifies a request and optional action under /api/request/.
type requestRoute struct {
	id     string
	action string
}

// requestRouteFromPath extracts a request route from path.
//
// Parameters:
//   - path: request URL path.
//
// Returns the route and true when the path has a non-empty request suffix.
func requestRouteFromPath(path string) (requestRoute, bool) {
	rest, ok := strings.CutPrefix(path, "/api/request/")
	if !ok || rest == "" {
		return requestRoute{}, false
	}
	id, action, _ := strings.Cut(rest, "/")
	return requestRoute{id: id, action: action}, true
}

// requestLimit picks the rate-limit budget for a request route: sending a
// reply uploads a body and shares the create budget; everything else is a
// read.
//
// Parameters:
//   - r: incoming request.
//
// Returns the limiter and its scope.
func (h *Handler) requestLimit(r *http.Request) (RateLimiter, rateScope) {
	route, _ := requestRouteFromPath(r.URL.Path)
	if route.action == "reply" && r.Method == http.MethodPut {
		return h.CreateLimiter, scopeCreate
	}
	return h.ReadLimiter, scopeRead
}

// singleValue returns the sole value of header name, or "" when it is
// absent or repeated, which the service rejects as an invalid token.
//
// Parameters:
//   - r: incoming request.
//   - name: canonical header name.
//
// Returns the header value or "".
func singleValue(r *http.Request, name string) string {
	vals := r.Header.Values(name)
	if len(vals) != 1 {
		return ""
	}
	return vals[0]
}

// handleCreateRequest implements POST /api/request. It carries X-Gone-TTL and
// no body, and responds 201 with the request ID, expiry, and both tokens.
// Tokens are never logged.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	if !h.requestsEnabled(w, r) || !h.allowMethod(w, r, http.MethodPost) {
		return
	}
	clog := requestLog(r)
	ttlStr, ok := singleHeader(r, "X-Gone-TTL")
	if !ok {
		h.writeError(r.Context(), w, http.StatusBadRequest, "missing required headers")
		return
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		h.writeError(r.Context(), w, http.StatusBadRequest, "invalid ttl")
		return
	}
	created, err := h.Requests.CreateRequest(r.Context(), ttl)
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Info("request create", "action", "error")
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		ID          string    `json:"id"`
		ExpiresAt   time.Time `json:"expires_at"`
		ManageToken string    `json:"manage_token"`
		FillToken   string    `json:"fill_token"`
	}{created.ID.String(), created.ExpiresAt.UTC(), created.ManageToken.String(), created.FillToken.String()})
	clog.Info("request create", "action", "success", "ttl_secs", int(ttl.Seconds()))
}

// handleRequestRoute dispatches /api/request/{id}[/{action}]:
//   - GET /api/request/{id} checks the request is open (fill token).
//   - PUT /api/request/{id}/reply sends the reply (fill token).
//   - GET /api/request/{id}/reply claims the reply (manage token).
//   - DELETE /api/request/{id}/reply acknowledges it (claim token).
//   - GET /api/request/{id}/status reports it to the requester.
//   - POST /api/request/{id}/revoke cancels it.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleRequestRoute(w http.ResponseWriter, r *http.Request) {
	route, ok := requestRouteFromPath(r.URL.Path)
	if !ok {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	if !h.requestsEnabled(w, r) {
		return
	}
	if route.action == "reply" {
		h.dispatchReply(w, r, route.id)
		return
	}
	a, ok := requestActions[route.action]
	if !ok {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	if h.allowMethod(w, r, a.method) {
		a.handle(h, w, r, route.id)
	}
}

// requestAction is a single-method request route.
type requestAction struct {
	method string
	handle func(h *Handler, w http.ResponseWriter, r *http.Request, id string)
}

// requestActions maps the single-method actions under /api/request/{id}.
var requestActions = map[string]requestAction{
	"":       {http.MethodGet, (*Handler).handleRequestOpen},
	"status": {http.MethodGet, (*Handler).handleRequestStatus},
	"revoke": {http.MethodPost, (*Handler).handleCancelRequest},
}

// dispatchReply routes /api/request/{id}/reply by method.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) dispatchReply(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPut:
		h.handleFillRequest(w, r, id)
	case http.MethodGet:
		h.handleClaimReply(w, r, id)
	case http.MethodDelete:
		h.handleAckReply(w, r, id)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		h.writeError(r.Context(), w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// requestsEnabled writes 404 and returns false when no RequestPort is wired.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//
// Returns true if request routes may be served.
func (h *Handler) requestsEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.Requests != nil {
		return true
	}
	h.writeError(r.Context(), w, http.StatusNotFound, "not found")
	return false
}

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
	res, err := h.Requests.ClaimReply(r.Context(), id, manageToken(r), r.Header.Get(HeaderClaim))
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
	if err := h.Requests.AckReply(r.Context(), id, r.Header.Get(HeaderClaim)); err != nil {
		h.mapServiceError(r.Context(), w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
	requestLog(r).Info("request ack", "action", "success")
}

// handleCancelRequest implements POST /api/request/{id}/revoke.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: request ID.
func (h *Handler) handleCancelRequest(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.Requests.CancelRequest(r.Context(), id, manageToken(r)); err != nil {
		h.mapServiceError(r.Context(), w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
	requestLog(r).Info("request cancel", "action", "success")
}

// requestLog returns a logger tagged with the request domain and correlation
// ID.
//
// Parameters:
//   - r: incoming request.
//
// Returns the logger.
func requestLog(r *http.Request) *slog.Logger {
	cid, _ := GetCorrelationID(r.Context())
	return slog.With("domain", "request", "cid", cid)
}

// writeJSON writes v as a no-store JSON response with status code.
//
// Parameters:
//   - w: response writer.
//   - code: HTTP status.
//   - v: value to encode.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// handleRequestPage serves the requester's page: /request (new request and
// the list) and /request/{id} (one request).
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleRequestPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/request" {
		h.renderRequestPage(w, r)
		return
	}
	h.serveIDPage(w, r, "/request/", h.RequestDetailTmpl, "request template unavailable")
}

// renderRequestPage renders /request with the configured TTL choices.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) renderRequestPage(w http.ResponseWriter, r *http.Request) {
	if h.RequestTmpl == nil {
		http.Error(w, "request template unavailable", http.StatusServiceUnavailable)
		return
	}
	renderTemplate(w, r, h.RequestTmpl, h.indexView())
}

// handleReplyPage serves the reply page at /reply/{id}. The page reads the
// public key and fill token from the URL fragment, which never reaches the
// server.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleReplyPage(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/reply/") || len(r.URL.Path) == len("/reply/") {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	if h.ReplyTmpl == nil {
		http.Error(w, "reply template unavailable", http.StatusServiceUnavailable)
		return
	}
	renderTemplate(w, r, h.ReplyTmpl, h.indexView())
}
