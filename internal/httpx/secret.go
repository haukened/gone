package httpx

import (
	"net/http"
	"strings"
)

// SecretRenderer abstracts template execution for the secret consumption and
// manage pages. It mirrors IndexRenderer/AboutRenderer to keep symmetry and
// simplify testing.
type SecretRenderer interface {
	Execute(w http.ResponseWriter, data any) error
}

// handleSecret serves the HTML page used to fetch and decrypt a one-time secret.
// It expects paths of the form /secret/{id}. A bare /secret/ (no ID) returns 404.
// The page itself performs client-side fetch & decrypt using the key fragment.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleSecret(w http.ResponseWriter, r *http.Request) {
	h.serveIDPage(w, r, "/secret/", h.SecretTmpl, "secret template unavailable")
}

// serveIDPage renders a client-side page addressed as {prefix}{id}. The ID is
// validated by the page's API calls, not here; a bare prefix returns 404.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - prefix: path prefix including the trailing slash.
//   - tmpl: page renderer; nil yields 503 with unavailable.
//   - unavailable: plain-text body used when tmpl is nil.
func (h *Handler) serveIDPage(w http.ResponseWriter, r *http.Request, prefix string, tmpl SecretRenderer, unavailable string) {
	if !strings.HasPrefix(r.URL.Path, prefix) || len(r.URL.Path) == len(prefix) {
		h.writeError(r.Context(), w, http.StatusNotFound, "not found")
		return
	}
	if tmpl == nil {
		http.Error(w, unavailable, http.StatusServiceUnavailable)
		return
	}
	renderTemplate(w, tmpl, struct{}{})
}
