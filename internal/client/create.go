package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/haukened/gone/internal/domain"
)

// CreateRequest is a sealed secret ready to store.
type CreateRequest struct {
	// Version is the protocol version the body was sealed with.
	Version uint8
	// Nonce is the AES-GCM nonce.
	Nonce []byte
	// TTL is the requested lifetime.
	TTL time.Duration
	// Body is the ciphertext (v1) or blob (v2).
	Body []byte
}

// CreateResult is the server's reply to a successful create.
type CreateResult struct {
	// ID is the new secret's ID.
	ID domain.SecretID
	// ExpiresAt is when the server will delete the secret.
	ExpiresAt time.Time
	// ManageToken is the sender's bearer token for status and revoke.
	ManageToken domain.ManageToken
}

// createResponse is the wire form of a create reply.
type createResponse struct {
	ID          string    `json:"id"`
	ExpiresAt   time.Time `json:"expires_at"`
	ManageToken string    `json:"manage_token"`
}

// Create stores a sealed secret (POST /api/secret).
//
// Parameters:
//   - ctx: request context.
//   - req: sealed secret and metadata.
//
// Returns the new secret's ID, expiry, and manage token, or a status,
// protocol, or network error.
func (c *Client) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	hdr := http.Header{}
	hdr.Set("X-Gone-Version", strconv.Itoa(int(req.Version)))
	hdr.Set("X-Gone-Nonce", domain.EncodeB64URL(req.Nonce))
	hdr.Set("X-Gone-TTL", req.TTL.String())
	body := req.Body
	if body == nil {
		body = []byte{}
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/secret", hdr, body)
	if err != nil {
		return CreateResult{}, err
	}
	if err = expect(resp, http.StatusCreated); err != nil {
		return CreateResult{}, err
	}
	raw, err := readBounded(resp, maxJSONBody)
	if err != nil {
		return CreateResult{}, err
	}
	return parseCreate(raw)
}

// parseCreate decodes and validates a create reply.
//
// Parameters:
//   - raw: response body.
//
// Returns the result, or ErrProtocol.
func parseCreate(raw []byte) (CreateResult, error) {
	var cr createResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return CreateResult{}, ErrProtocol
	}
	id, err := domain.ParseID(cr.ID)
	if err != nil {
		return CreateResult{}, ErrProtocol
	}
	tok, err := domain.ParseManageToken(cr.ManageToken)
	if err != nil || cr.ExpiresAt.IsZero() {
		return CreateResult{}, ErrProtocol
	}
	return CreateResult{ID: id, ExpiresAt: cr.ExpiresAt, ManageToken: tok}, nil
}
