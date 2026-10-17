package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-jose/go-jose/v4"
)

// httpsOnly refuses redirects away from https.
func httpsOnly(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" || len(via) >= 5 {
		return errors.New("refusing redirect")
	}
	return nil
}

// fetch resolves the key set URL (through discovery when needed) and reads
// the public keys from it.
//
// Parameters:
//   - ctx: cancellation.
//
// Returns the keys and the key set URL, or an error.
func (k *keySource) fetch(ctx context.Context) ([]jose.JSONWebKey, string, error) {
	keysURL := k.url
	if k.discover {
		var err error
		if keysURL, err = k.discoverKeys(ctx); err != nil {
			return nil, "", err
		}
	}
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := k.getJSON(ctx, keysURL, &set); err != nil {
		return nil, "", err
	}
	keys := publicKeys(set.Keys)
	if len(keys) == 0 {
		return nil, "", errors.New("the key set has no usable public keys")
	}
	return keys, keysURL, nil
}

// discoverKeys reads the OpenID discovery document and returns its
// jwks_uri, after checking its issuer matches GONE_AUTH_ISSUER.
//
// Parameters:
//   - ctx: cancellation.
//
// Returns the key set URL, or an error.
func (k *keySource) discoverKeys(ctx context.Context) (string, error) {
	var doc struct {
		Issuer  string `json:"issuer"`
		KeysURI string `json:"jwks_uri"`
	}
	if err := k.getJSON(ctx, k.url, &doc); err != nil {
		return "", err
	}
	if doc.Issuer != k.issuer {
		return "", fmt.Errorf("discovery issuer %q does not match GONE_AUTH_ISSUER", doc.Issuer)
	}
	if u, err := url.Parse(doc.KeysURI); err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("discovery jwks_uri is not an https URL")
	}
	return doc.KeysURI, nil
}

// getJSON fetches an https URL and decodes at most maxKeysBody bytes of
// JSON into dst.
//
// Parameters:
//   - ctx: cancellation.
//   - rawURL: the URL.
//   - dst: the value to decode into.
//
// Returns an error for a failed request, a non-200 status, an oversized
// body or bad JSON.
func (k *keySource) getJSON(ctx context.Context, rawURL string, dst any) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil || req.URL.Scheme != "https" {
		return errors.New("not an https URL")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := k.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxKeysBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxKeysBody {
		return errors.New("response larger than 1 MiB")
	}
	return json.Unmarshal(body, dst)
}

// publicKeys parses each key on its own, keeping valid public signing keys
// and skipping any it can't use, so one odd entry doesn't break the set.
//
// Parameters:
//   - raw: the "keys" array members.
//
// Returns the usable keys.
func publicKeys(raw []json.RawMessage) []jose.JSONWebKey {
	var out []jose.JSONWebKey
	for _, r := range raw {
		var key jose.JSONWebKey
		if json.Unmarshal(r, &key) != nil || !key.Valid() || !key.IsPublic() || (key.Use != "" && key.Use != "sig") {
			continue
		}
		out = append(out, key)
	}
	return out
}
