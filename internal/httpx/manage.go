package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// HeaderManage carries the sender's manage token on status and revoke
// requests. It must appear exactly once; the token is never logged.
const HeaderManage = "X-Gone-Manage"

// manageToken returns the single X-Gone-Manage header value. A missing or
// repeated header yields "", which the service rejects as an invalid token.
//
// Parameters:
//   - r: incoming request.
//
// Returns the header value, or "" when it is absent or duplicated.
func manageToken(r *http.Request) string {
	vals := r.Header.Values(HeaderManage)
	if len(vals) != 1 {
		return ""
	}
	return vals[0]
}

// statusResponse is the JSON body for a pending secret's status.
type statusResponse struct {
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleStatusSecret implements GET /api/secret/{id}/status. It returns 200
// with the secret's timestamps while it is pending; opened, revoked, expired,
// unknown, and wrong-token secrets all return the same 404.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: secret ID extracted from the path.
func (h *Handler) handleStatusSecret(w http.ResponseWriter, r *http.Request, id string) {
	cid, _ := GetCorrelationID(r.Context())
	clog := slog.With("domain", "secret", "cid", cid)
	st, err := h.Service.Status(r.Context(), id, manageToken(r))
	if err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Info("status", "action", "miss")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(statusResponse{
		State:     "pending",
		CreatedAt: st.CreatedAt.UTC(),
		ExpiresAt: st.ExpiresAt.UTC(),
	})
	clog.Info("status", "action", "pending")
}

// handleRevokeSecret implements POST /api/secret/{id}/revoke. It permanently
// deletes a pending secret and returns 204, or the uniform 404 otherwise.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
//   - id: secret ID extracted from the path.
func (h *Handler) handleRevokeSecret(w http.ResponseWriter, r *http.Request, id string) {
	cid, _ := GetCorrelationID(r.Context())
	clog := slog.With("domain", "secret", "cid", cid)
	clog.Info("revoke", "action", "start")
	if err := h.Service.Revoke(r.Context(), id, manageToken(r)); err != nil {
		h.mapServiceError(r.Context(), w, err)
		clog.Info("revoke", "action", "miss")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
	clog.Info("revoke", "action", "success")
}

// handleManage serves the sender's manage page at /manage/{id}. The page reads
// the manage token from the URL fragment, which never reaches the server.
//
// Parameters:
//   - w: response writer.
//   - r: incoming request.
func (h *Handler) handleManage(w http.ResponseWriter, r *http.Request) {
	h.serveIDPage(w, r, "/manage/", h.ManageTmpl, "manage template unavailable")
}
