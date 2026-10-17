package httpx

import (
	"log/slog"
	"net/http"

	"github.com/haukened/gone/v3/internal/metrics"
)

// requireSignIn wraps a create handler (POST /api/secret, POST
// /api/request): when sign-in is on, a POST without valid proof of sign-in
// gets 401 before the body is read. Other methods pass through, so they
// still get the handler's own 405.
//
// Parameters:
//   - next: the create handler.
//
// Returns the wrapped handler.
func (h *Handler) requireSignIn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.Auth == nil || r.Method != http.MethodPost || h.Auth.SignedIn(r) {
			next(w, r)
			return
		}
		cid, _ := GetCorrelationID(r.Context())
		slog.Info("create", "cid", cid, "action", "error", "kind", "sign_in")
		if h.Metrics != nil {
			h.Metrics.Inc(metrics.CounterSignInDenied, 1) // records nothing about who
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="gone"`)
		h.writeError(r.Context(), w, http.StatusUnauthorized, "sign in required")
	}
}

// needsSignIn reports whether a create page should show the sign-in view
// instead of its form, and the sign-in button's target.
//
// Parameters:
//   - r: the page request.
//   - page: the page's own path, "/" or "/request".
//
// Returns whether to show the view, and the button URL ("" for none).
func (h *Handler) needsSignIn(r *http.Request, page string) (bool, string) {
	if h.Auth == nil || h.Auth.SignedIn(r) {
		return false, ""
	}
	return true, h.Auth.SignInURL(page)
}
