package auth

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
)

const (
	// fetchTimeout bounds one key or discovery request.
	fetchTimeout = 10 * time.Second
	// maxKeysBody caps a key set or discovery document.
	maxKeysBody = 1 << 20
	// refreshEvery is how often loaded keys are refreshed.
	refreshEvery = time.Hour
	// refetchAfter rate-limits refetches for unknown key IDs, so forged IDs
	// can't make Gone hammer the provider.
	refetchAfter = 5 * time.Minute
	// maxRetry caps the wait between failed initial loads.
	maxRetry = time.Minute
)

// keySource fetches and caches a provider's public signing keys.
type keySource struct {
	client   *http.Client
	url      string // key set, or the discovery document when discover is set
	discover bool
	issuer   string // expected issuer, checked against discovery
	now      func() time.Time
	retry    time.Duration // first wait after a failed initial load

	fetchMu   sync.Mutex // serializes fetches
	mu        sync.RWMutex
	keys      []jose.JSONWebKey
	keysURL   string // resolved key set URL
	fetchedAt time.Time
}

// newKeySource prepares a key source for a provider's endpoints.
//
// Parameters:
//   - ep: the provider's resolved endpoints.
//   - client: HTTP client; nil means one with fetchTimeout that only
//     follows redirects to https.
//
// Returns the key source; nothing is fetched yet.
func newKeySource(ep endpoints, client *http.Client) *keySource {
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout, CheckRedirect: httpsOnly}
	}
	return &keySource{client: client, url: ep.keysURL, discover: ep.discover, issuer: ep.issuer, now: time.Now, retry: time.Second}
}

// run loads the keys, retrying with backoff until it succeeds, then
// refreshes them every hour until ctx ends. Until the first load succeeds
// nobody is signed in.
//
// Parameters:
//   - ctx: canceling it stops the loop.
func (k *keySource) run(ctx context.Context) {
	for wait := k.retry; k.load(ctx) != nil; wait = min(wait*2, maxRetry) {
		if !sleep(ctx, wait) {
			return
		}
	}
	for sleep(ctx, refreshEvery) {
		_ = k.load(ctx)
	}
}

// sleep waits for d or until ctx ends.
//
// Returns false when ctx ended.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// load fetches the key set and replaces the cached keys. A failure keeps the
// last good keys and is logged without any request data.
//
// Parameters:
//   - ctx: cancellation.
//
// Returns the fetch error, if any.
func (k *keySource) load(ctx context.Context) error {
	k.fetchMu.Lock()
	defer k.fetchMu.Unlock()
	return k.store(ctx)
}

// refetch is load for an unknown key ID: requests that queued behind
// another refetch reuse its result instead of fetching again, so a burst of
// forged key IDs costs the provider one request.
//
// Parameters:
//   - ctx: cancellation.
//   - seen: when the keys were last fetched, as this request saw it.
//
// Returns the fetch error, if any; nil when another request just fetched.
func (k *keySource) refetch(ctx context.Context, seen time.Time) error {
	k.fetchMu.Lock()
	defer k.fetchMu.Unlock()
	k.mu.RLock()
	fetched := k.fetchedAt.After(seen)
	k.mu.RUnlock()
	if fetched {
		return nil
	}
	return k.store(ctx)
}

// store fetches the keys and records the attempt. The caller holds
// k.fetchMu.
//
// Parameters:
//   - ctx: cancellation.
//
// Returns the fetch error, if any.
func (k *keySource) store(ctx context.Context) error {
	keys, keysURL, err := k.fetch(ctx)
	k.mu.Lock()
	k.fetchedAt = k.now()
	if err == nil {
		k.keys, k.keysURL = keys, keysURL
	}
	kept := len(k.keys) > 0
	k.mu.Unlock()
	switch {
	case err == nil:
	case kept:
		slog.Warn("sign-in: could not refresh the provider's signing keys; keeping the last ones loaded", "url", k.url, "err", err)
	default:
		slog.Warn("sign-in: could not load the provider's signing keys; creating is refused until they load", "url", k.url, "err", err)
	}
	return err
}

// lookup returns the public key for a token's key ID and algorithm. An
// unknown ID triggers one refetch, at most once per refetchAfter, which
// isn't cancelled with the request.
//
// Parameters:
//   - ctx: request context, for a refetch.
//   - kid: the token's key ID; "" matches only a set with one key.
//   - alg: the provider's pinned algorithm.
//
// Returns the key, or nil when none fits.
func (k *keySource) lookup(ctx context.Context, kid string, alg jose.SignatureAlgorithm) any {
	if key := k.find(kid, alg); key != nil {
		return key
	}
	k.mu.RLock()
	seen := k.fetchedAt
	k.mu.RUnlock()
	// The refetch outlives the request: a client that hangs up mustn't cancel
	// it, or a forged key ID could keep rotated keys from ever loading.
	if k.now().Sub(seen) < refetchAfter || k.refetch(context.WithoutCancel(ctx), seen) != nil {
		return nil
	}
	return k.find(kid, alg)
}

// find searches the cached keys.
//
// Parameters:
//   - kid: the key ID; "" matches only a set with one key.
//   - alg: the pinned algorithm; a key that names another is skipped.
//
// Returns the public key, or nil.
func (k *keySource) find(kid string, alg jose.SignatureAlgorithm) any {
	k.mu.RLock()
	defer k.mu.RUnlock()
	for _, key := range k.keys {
		if k.idMatches(key, kid) && (key.Algorithm == "" || key.Algorithm == string(alg)) {
			return key.Key
		}
	}
	return nil
}

// idMatches reports whether key fits a token's key ID. A token without one
// fits only a set with exactly one key. The caller holds k.mu.
//
// Parameters:
//   - key: a cached key.
//   - kid: the token's key ID.
//
// Returns true when the key may verify the token.
func (k *keySource) idMatches(key jose.JSONWebKey, kid string) bool {
	if kid == "" {
		return len(k.keys) == 1
	}
	return key.KeyID == kid
}
