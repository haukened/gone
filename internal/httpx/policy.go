package httpx

import "net/http"

// policyView is the JSON body of GET /api/policy.
type policyView struct {
	PassphraseRequired bool `json:"passphrase_required"`
	MaxFiles           int  `json:"max_files"`
}

// handlePolicy implements GET /api/policy: the operator policy clients honour
// before they encrypt. The server enforces passphrase_required on
// POST /api/secret; max_files is advisory, because the file count is inside
// the ciphertext. Like the other API routes it answers its one method, GET,
// and it is never cached, so a changed setting applies on the next send.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handlePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		h.writeError(r.Context(), w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, policyView{PassphraseRequired: h.RequirePassphrase, MaxFiles: h.MaxFiles})
}
