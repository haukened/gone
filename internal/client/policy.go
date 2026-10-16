package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// Policy is the operator policy a server publishes at GET /api/policy.
type Policy struct {
	// PassphraseRequired means POST /api/secret accepts only v2 secrets.
	PassphraseRequired bool `json:"passphrase_required"`
	// MaxFiles is how many files a secret or reply may carry; 0 means none.
	// The server cannot check it (the file count is encrypted), so clients
	// honour it before sealing.
	MaxFiles int `json:"max_files"`
}

// Policy fetches the server's operator policy (GET /api/policy).
//
// Parameters:
//   - ctx: request context.
//
// Returns the policy, ErrNotFound from a server older than v3.12 that has
// no policy route, or a status, protocol, or network error.
func (c *Client) Policy(ctx context.Context) (Policy, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/policy", nil, nil)
	if err != nil {
		return Policy{}, err
	}
	if err = expect(resp, http.StatusOK); err != nil {
		return Policy{}, err
	}
	raw, err := readBounded(resp, maxJSONBody)
	if err != nil {
		return Policy{}, err
	}
	var p struct {
		PassphraseRequired *bool `json:"passphrase_required"`
		MaxFiles           *int  `json:"max_files"`
	}
	if json.Unmarshal(raw, &p) != nil || p.PassphraseRequired == nil || p.MaxFiles == nil || *p.MaxFiles < 0 {
		return Policy{}, ErrProtocol
	}
	return Policy{PassphraseRequired: *p.PassphraseRequired, MaxFiles: *p.MaxFiles}, nil
}
