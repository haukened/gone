package httpx

import (
	"net/http"
	"strings"
)

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
	view := h.indexView()
	view.NeedsSignIn, view.SignInURL = h.needsSignIn(r, "/request")
	renderTemplate(w, r, h.RequestTmpl, view)
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
		h.renderErrorPage(w, r, http.StatusNotFound, "error.notFound")
		return
	}
	if h.ReplyTmpl == nil {
		http.Error(w, "reply template unavailable", http.StatusServiceUnavailable)
		return
	}
	renderTemplate(w, r, h.ReplyTmpl, h.indexView())
}
