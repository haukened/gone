package client

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// Response and request header names (docs/protocol.md §8).
const (
	headerVersion      = "X-Gone-Version"
	headerNonce        = "X-Gone-Nonce"
	headerClaim        = "X-Gone-Claim"
	headerClaimExpires = "X-Gone-Claim-Expires"
	headerManage       = "X-Gone-Manage"
	headerFill         = "X-Gone-Fill"
)

// Claimed is a fetched, not yet acknowledged, secret.
type Claimed struct {
	// Nonce is the decoded AES-GCM nonce.
	Nonce []byte
	// Token acknowledges the claim; it is a bearer secret.
	Token domain.ClaimToken
	// Expires is the end of the claim lease.
	Expires time.Time
	// Body is the ciphertext or v2 blob. Callers should clear it after use.
	Body []byte
}

// claimRetries bounds how often an interrupted ciphertext download is
// fetched again with the claim token (docs/protocol.md §8.2).
const claimRetries = 2

// Claim fetches a secret's ciphertext and opens a claim lease
// (GET /api/secret/{id}). The secret is not deleted until Ack. An
// interrupted download is fetched again with the claim token while the
// lease lasts.
//
// Parameters:
//   - ctx: request context.
//   - id: secret ID.
//   - version: the link's protocol version; the response must match it.
//
// Returns the claimed secret, or ErrVersionMismatch, ErrIntegrity,
// ErrProtocol, ErrTooLarge, or a status or network error.
func (c *Client) Claim(ctx context.Context, id domain.SecretID, version uint8) (Claimed, error) {
	return c.claim(ctx, secretPath+id.String(), http.Header{}, version, domain.ValidateProtocol)
}

// claim GETs a claim and, when the body download fails after the claim
// token arrived, fetches it again with X-Gone-Claim, at most claimRetries
// times and only while the lease is valid. Without the retry the claim
// would already have started the secret's deletion.
//
// Parameters:
//   - ctx: request context.
//   - path: secret or reply path.
//   - hdr: request headers for the first GET (not modified).
//   - version: protocol version the caller expects.
//   - validate: checks the version and nonce for this route.
//
// Returns the claimed ciphertext or the last error.
func (c *Client) claim(ctx context.Context, path string, hdr http.Header, version uint8, validate func(uint8, string) error) (Claimed, error) {
	first, err := c.fetchClaim(ctx, path, hdr, version, validate)
	got := first
	for i := 0; i < claimRetries && errors.Is(err, ErrNetwork) && first.Token != "" && time.Now().Before(first.Expires); i++ {
		retry := hdr.Clone()
		retry.Set(headerClaim, first.Token.String())
		got, err = c.fetchClaim(ctx, path, retry, version, validate)
	}
	if err != nil {
		return Claimed{}, err
	}
	return got, nil
}

// fetchClaim sends one claim GET and reads the response.
//
// Parameters:
//   - ctx: request context.
//   - path: secret or reply path.
//   - hdr: request headers.
//   - version: protocol version the caller expects.
//   - validate: checks the version and nonce for this route.
//
// Returns the claim; on a body read failure, the claim headers (token and
// lease) with the error.
func (c *Client) fetchClaim(ctx context.Context, path string, hdr http.Header, version uint8, validate func(uint8, string) error) (Claimed, error) {
	resp, err := c.do(ctx, http.MethodGet, path, hdr, nil)
	if err != nil {
		return Claimed{}, err
	}
	if err = expect(resp, http.StatusOK); err != nil {
		return Claimed{}, err
	}
	return readClaim(resp, version, validate)
}

// readClaim checks a 200 claim response and reads its ciphertext.
//
// Parameters:
//   - resp: the claim response (status already checked).
//   - version: protocol version the caller expects.
//   - validate: checks the version and nonce for this route.
//
// Returns the claimed ciphertext, or ErrVersionMismatch, ErrIntegrity,
// ErrProtocol, ErrTooLarge, or a read error together with the parsed
// claim headers.
func readClaim(resp *http.Response, version uint8, validate func(uint8, string) error) (Claimed, error) {
	cl, err := parseClaimHeaders(resp.Header, version, validate)
	if err != nil {
		discard(resp)
		return Claimed{}, err
	}
	cl.Body, err = readBounded(resp, MaxCiphertext)
	return cl, err
}

// parseClaimHeaders validates the protocol headers of a claim response in
// the order docs/protocol.md §8.2 requires: version, then nonce.
//
// Parameters:
//   - h: response headers.
//   - version: expected protocol version.
//
// Returns the claim metadata without a body, or an error.
func parseClaimHeaders(h http.Header, version uint8, validate func(uint8, string) error) (Claimed, error) {
	v, ok := single(h, headerVersion)
	if !ok || v != strconv.Itoa(int(version)) {
		return Claimed{}, ErrVersionMismatch
	}
	n, ok := single(h, headerNonce)
	if !ok || validate(version, n) != nil {
		return Claimed{}, ErrIntegrity
	}
	nonce, _ := domain.DecodeB64URL(n) // validated above
	tv, _ := single(h, headerClaim)
	tok, err := domain.ParseClaimToken(tv)
	if err != nil {
		return Claimed{}, ErrProtocol
	}
	ev, _ := single(h, headerClaimExpires)
	exp, err := time.Parse(time.RFC3339, ev)
	if err != nil {
		return Claimed{}, ErrProtocol
	}
	return Claimed{Nonce: nonce, Token: tok, Expires: exp}, nil
}

// Ack acknowledges a claim, permanently deleting the secret
// (DELETE /api/secret/{id}).
//
// Parameters:
//   - ctx: request context.
//   - id: secret ID.
//   - token: claim token from Claim.
//
// Returns nil on success, including a 404 (the row is already deleted
// because the lease lapsed or the secret was revoked), or another status or
// network error.
func (c *Client) Ack(ctx context.Context, id domain.SecretID, token domain.ClaimToken) error {
	return c.ack(ctx, secretPath+id.String(), token)
}

// ack sends DELETE path with the claim token; a 404 means the ciphertext is
// already deleted, which counts as success.
//
// Parameters:
//   - ctx: request context.
//   - path: secret or reply path.
//   - token: claim token.
//
// Returns nil or a client error.
func (c *Client) ack(ctx context.Context, path string, token domain.ClaimToken) error {
	hdr := http.Header{}
	hdr.Set(headerClaim, token.String())
	resp, err := c.do(ctx, http.MethodDelete, path, hdr, nil)
	if err != nil {
		return err
	}
	err = expect(resp, http.StatusNoContent)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	discard(resp)
	return nil
}
