package client

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// Status describes a pending secret.
type Status struct {
	// State is always "pending"; every other state is ErrNotFound.
	State string
	// CreatedAt is when the secret was stored.
	CreatedAt time.Time
	// ExpiresAt is when the server will delete the secret.
	ExpiresAt time.Time
}

// statusResponse is the wire form of a status reply.
type statusResponse struct {
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Status reports whether a secret is still pending
// (GET /api/secret/{id}/status). It never claims or deletes the secret.
//
// Parameters:
//   - ctx: request context.
//   - id: secret ID.
//   - token: the sender's manage token.
//
// Returns the status, ErrNotFound when the secret is gone for any reason,
// or another status, protocol, or network error.
func (c *Client) Status(ctx context.Context, id domain.SecretID, token domain.ManageToken) (Status, error) {
	//nolint:bodyclose // readBounded closes the successful response body.
	resp, err := c.manage(ctx, http.MethodGet, secretPath, id, "/status", token, http.StatusOK)
	if err != nil {
		return Status{}, err
	}
	raw, err := readBounded(resp, maxJSONBody)
	if err != nil {
		return Status{}, err
	}
	var sr statusResponse
	if err = json.Unmarshal(raw, &sr); err != nil || sr.State != "pending" {
		return Status{}, ErrProtocol
	}
	return Status(sr), nil
}

// Revoke permanently deletes a pending secret
// (POST /api/secret/{id}/revoke).
//
// Parameters:
//   - ctx: request context.
//   - id: secret ID.
//   - token: the sender's manage token.
//
// Returns nil on success, ErrNotFound when the secret is already gone, or
// another status or network error.
func (c *Client) Revoke(ctx context.Context, id domain.SecretID, token domain.ManageToken) error {
	resp, err := c.manage(ctx, http.MethodPost, secretPath, id, "/revoke", token, http.StatusNoContent)
	if err != nil {
		return err
	}
	discard(resp)
	return nil
}

// secretPath and requestPath prefix the per-secret and per-request routes.
const (
	secretPath  = "/api/secret/"
	requestPath = "/api/request/"
)

// manage sends a manage-token request and checks its status.
//
// Parameters:
//   - ctx: request context.
//   - method: HTTP method.
//   - prefix: secretPath or requestPath.
//   - id: secret ID.
//   - suffix: path suffix after the ID.
//   - token: manage token.
//   - want: expected status code.
//
// Returns the open response on success, or an error.
func (c *Client) manage(ctx context.Context, method, prefix string, id domain.SecretID, suffix string, token domain.ManageToken, want int) (*http.Response, error) {
	hdr := http.Header{}
	hdr.Set(headerManage, token.String())
	resp, err := c.do(ctx, method, prefix+id.String()+suffix, hdr, nil)
	if err != nil {
		return nil, err
	}
	if err = expect(resp, want); err != nil {
		return nil, err
	}
	return resp, nil
}
