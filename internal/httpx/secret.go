package httpx

import (
	"net/http"
	"strings"
)

// SecretRenderer abstracts template execution for the secret consumption page.
// It mirrors IndexRenderer/AboutRenderer to keep symmetry and simplify testing.
type SecretRenderer interface {
	Execute(w http.ResponseWriter, data any) error
}

// handleSecret serves the HTML page used to fetch and decrypt a one-time secret.
// It expects paths of the form /secret/{id}. A bare /secret/ (no ID) returns 404.
// The page itself performs client-side fetch & decrypt using the key fragment.
func (h *Handler) handleSecret(w http.ResponseWriter, r *http.Request) {
	const prefix = "/secret/"
	if !strings.HasPrefix(r.URL.Path, prefix) || len(r.URL.Path) == len(prefix) { // no id present
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}
	if h.SecretTmpl == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("secret template unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Minimal data today; future fields could include feature flags.
	if err := h.SecretTmpl.Execute(w, struct{}{}); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("template error"))
	}
}
