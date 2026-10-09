package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// CreatedRequest is returned by CreateRequest.
type CreatedRequest struct {
	// ID is the request ID.
	ID domain.SecretID
	// ExpiresAt is the deadline for a reply.
	ExpiresAt time.Time
	// ManageToken authorizes status, claiming the reply, and cancel. Keep it
	// private; it never belongs in a link.
	ManageToken domain.ManageToken
	// FillToken goes in the reply link and authorizes the one reply.
	FillToken domain.FillToken
}

// RequestState reports a request to its requester.
type RequestState struct {
	// Ready is true once a reply is stored and not yet opened.
	Ready bool
	// CreatedAt is when the request (or, once ready, its reply) was stored.
	CreatedAt time.Time
	// ExpiresAt is the deadline for a reply, or for opening it once ready.
	ExpiresAt time.Time
}

type createdRequestResponse struct {
	ID          string    `json:"id"`
	ExpiresAt   time.Time `json:"expires_at"`
	ManageToken string    `json:"manage_token"`
	FillToken   string    `json:"fill_token"`
}

type expiresResponse struct {
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateRequest opens a secret request (docs/protocol.md section 8.6). The
// server learns only ttl; the key pair stays with the caller.
//
// Parameters:
//   - ctx: request context.
//   - ttl: how long to wait for a reply; a reply is then kept as long again.
//
// Returns the new request, or a StatusError, RateLimitError, ErrProtocol or
// ErrNetwork.
func (c *Client) CreateRequest(ctx context.Context, ttl time.Duration) (CreatedRequest, error) {
	hdr := http.Header{}
	hdr.Set("X-Gone-TTL", ttl.String())
	resp, err := c.do(ctx, http.MethodPost, "/api/request", hdr, nil)
	if err != nil {
		return CreatedRequest{}, err
	}
	if err = expect(resp, http.StatusCreated); err != nil {
		return CreatedRequest{}, err
	}
	raw, err := readBounded(resp, maxJSONBody)
	if err != nil {
		return CreatedRequest{}, err
	}
	return parseCreatedRequest(raw)
}

// parseCreatedRequest validates a create-request response body.
//
// Parameters:
//   - raw: JSON body.
//
// Returns the request or ErrProtocol.
func parseCreatedRequest(raw []byte) (CreatedRequest, error) {
	var cr createdRequestResponse
	if err := json.Unmarshal(raw, &cr); err != nil || cr.ExpiresAt.IsZero() {
		return CreatedRequest{}, ErrProtocol
	}
	id, idErr := domain.ParseID(cr.ID)
	manage, mErr := domain.ParseManageToken(cr.ManageToken)
	fill, fErr := domain.ParseFillToken(cr.FillToken)
	if err := errors.Join(idErr, mErr, fErr); err != nil {
		return CreatedRequest{}, ErrProtocol
	}
	return CreatedRequest{ID: id, ExpiresAt: cr.ExpiresAt, ManageToken: manage, FillToken: fill}, nil
}

// RequestOpen checks that a request can still be answered. It changes
// nothing on the server.
//
// Parameters:
//   - ctx: request context.
//   - id: request ID.
//   - fill: fill token from the reply link.
//
// Returns the reply deadline, or ErrNotFound when the request is not open.
func (c *Client) RequestOpen(ctx context.Context, id domain.SecretID, fill domain.FillToken) (time.Time, error) {
	hdr := http.Header{}
	hdr.Set(headerFill, fill.String())
	resp, err := c.do(ctx, http.MethodGet, requestPath+id.String(), hdr, nil)
	if err != nil {
		return time.Time{}, err
	}
	if err = expect(resp, http.StatusOK); err != nil {
		return time.Time{}, err
	}
	st, err := readExpires(resp)
	if err != nil || st.State != "open" {
		return time.Time{}, ErrProtocol
	}
	return st.ExpiresAt, nil
}

// Fill sends the one reply to a request: a protocol v3 ciphertext.
//
// Parameters:
//   - ctx: request context.
//   - id: request ID.
//   - fill: fill token from the reply link.
//   - nonce: 12-byte nonce.
//   - body: pubE || ciphertext || tag.
//
// Returns the reply's expiry, or ErrNotFound when the request is no longer
// open, or another client error.
func (c *Client) Fill(ctx context.Context, id domain.SecretID, fill domain.FillToken, nonce, body []byte) (time.Time, error) {
	hdr := http.Header{}
	hdr.Set(headerVersion, strconv.Itoa(int(domain.ProtocolV3)))
	hdr.Set(headerNonce, domain.EncodeB64URL(nonce))
	hdr.Set(headerFill, fill.String())
	resp, err := c.do(ctx, http.MethodPut, requestPath+id.String()+"/reply", hdr, body)
	if err != nil {
		return time.Time{}, err
	}
	if err = expect(resp, http.StatusCreated); err != nil {
		return time.Time{}, err
	}
	st, err := readExpires(resp)
	if err != nil {
		return time.Time{}, ErrProtocol
	}
	return st.ExpiresAt, nil
}

// RequestStatus reports a request to its requester. It changes nothing.
//
// Parameters:
//   - ctx: request context.
//   - id: request ID.
//   - token: the request's manage token.
//
// Returns the state, or ErrNotFound when the request is gone.
func (c *Client) RequestStatus(ctx context.Context, id domain.SecretID, token domain.ManageToken) (RequestState, error) {
	resp, err := c.manage(ctx, http.MethodGet, requestPath, id, "/status", token, http.StatusOK)
	if err != nil {
		return RequestState{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	st, err := readExpires(resp)
	if err != nil || (st.State != "waiting" && st.State != "ready") || st.CreatedAt.IsZero() {
		return RequestState{}, ErrProtocol
	}
	return RequestState{Ready: st.State == "ready", CreatedAt: st.CreatedAt, ExpiresAt: st.ExpiresAt}, nil
}

// ClaimReply claims a ready reply for the requester and returns its
// ciphertext without deleting it; Ack it with AckReply once opened.
//
// Parameters:
//   - ctx: request context.
//   - id: request ID.
//   - token: the request's manage token.
//
// Returns the claimed reply, or ErrNotFound, ErrVersionMismatch,
// ErrIntegrity, ErrProtocol, ErrTooLarge, or a network error.
func (c *Client) ClaimReply(ctx context.Context, id domain.SecretID, token domain.ManageToken) (Claimed, error) {
	resp, err := c.manage(ctx, http.MethodGet, requestPath, id, "/reply", token, http.StatusOK)
	if err != nil {
		return Claimed{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	return readClaim(resp, domain.ProtocolV3, domain.ValidateReplyProtocol)
}

// AckReply confirms the reply was opened, deleting it. A 404 means it is
// already deleted, which is success.
//
// Parameters:
//   - ctx: request context.
//   - id: request ID.
//   - claim: claim token from ClaimReply.
//
// Returns nil, or a client error.
func (c *Client) AckReply(ctx context.Context, id domain.SecretID, claim domain.ClaimToken) error {
	return c.ack(ctx, requestPath+id.String()+"/reply", claim)
}

// CancelRequest deletes an open request or an unopened reply.
//
// Parameters:
//   - ctx: request context.
//   - id: request ID.
//   - token: the request's manage token.
//
// Returns nil, ErrNotFound when there was nothing to cancel, or another
// client error.
func (c *Client) CancelRequest(ctx context.Context, id domain.SecretID, token domain.ManageToken) error {
	resp, err := c.manage(ctx, http.MethodPost, requestPath, id, "/revoke", token, http.StatusNoContent)
	if err != nil {
		return err
	}
	discard(resp)
	return nil
}

// readExpires decodes a small JSON body carrying state and timestamps.
//
// Parameters:
//   - resp: response with a JSON body.
//
// Returns the decoded body, or ErrProtocol or a read error.
func readExpires(resp *http.Response) (expiresResponse, error) {
	raw, err := readBounded(resp, maxJSONBody)
	if err != nil {
		return expiresResponse{}, err
	}
	var st expiresResponse
	if err = json.Unmarshal(raw, &st); err != nil || st.ExpiresAt.IsZero() {
		return expiresResponse{}, ErrProtocol
	}
	return st, nil
}
