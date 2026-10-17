package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/haukened/gone/v3/internal/ratelimit"
)

const (
	// maxSignedHeader caps a signed token's header value.
	maxSignedHeader = 8 << 10
	// maxPlainHeader caps a plain identity header's value.
	maxPlainHeader = 512
	// leeway allows for clock differences between Gone and the proxy.
	leeway = 60 * time.Second
	// returnMarker in GONE_AUTH_SIGNIN_URL is replaced with the page's path.
	returnMarker = "{return}"
)

// Gate decides whether a request is signed in. A nil *Gate means sign-in
// is off; callers check for nil before asking.
type Gate struct {
	header    string
	plain     *plainCheck // set for header providers
	signed    *tokenCheck // set for signed providers
	signInURL string
}

// plainCheck accepts a plain identity header from a trusted proxy.
type plainCheck struct {
	trusted []netip.Prefix
	format  string // "any", "email" or "exact"
	exact   []byte
}

// tokenCheck verifies a signed token with the provider's keys.
type tokenCheck struct {
	alg      jose.SignatureAlgorithm
	issuer   string
	audience string
	keys     *keySource
	now      func() time.Time
}

// New builds the gate for validated settings.
//
// Parameters:
//   - s: the settings, already checked by Validate.
//   - trusted: GONE_TRUSTED_PROXIES.
//   - client: HTTP client for fetching keys; nil means a default with a
//     10-second timeout.
//
// Returns the gate, or nil when sign-in is off.
func New(s Settings, trusted []netip.Prefix, client *http.Client) *Gate {
	if !s.Enabled() {
		return nil
	}
	sp := specs[s.Provider]
	g := &Gate{header: orDefault(orDefault(s.Header, sp.header), "Authorization"), signInURL: s.SignInURL}
	if !sp.signed {
		g.plain = &plainCheck{trusted: trusted, format: orDefault(s.Format, "any"), exact: []byte(s.Value)}
		return g
	}
	alg := sp.alg
	if s.Alg != "" {
		alg = jose.SignatureAlgorithm(s.Alg)
	}
	ep := sp.resolve(s)
	g.signed = &tokenCheck{alg: alg, issuer: ep.issuer, audience: ep.audience, keys: newKeySource(ep, client), now: time.Now}
	return g
}

// Start loads the signing keys and keeps them fresh until ctx ends. It
// returns at once; header providers have nothing to load.
//
// Parameters:
//   - ctx: canceling it stops the refresh loop.
func (g *Gate) Start(ctx context.Context) {
	if g != nil && g.signed != nil {
		go g.signed.keys.run(ctx)
	}
}

// SignedIn reports whether the request carries proof of sign-in.
//
// Parameters:
//   - r: the request.
//
// Returns true when the header is valid for the provider.
func (g *Gate) SignedIn(r *http.Request) bool {
	values := r.Header.Values(g.header)
	if len(values) != 1 {
		return false
	}
	v := strings.TrimSpace(values[0])
	if g.plain != nil {
		return len(v) <= maxPlainHeader && g.plain.ok(r.RemoteAddr, v)
	}
	return len(v) <= maxSignedHeader && g.signed.ok(r.Context(), stripBearer(v))
}

// SignInURL returns the sign-in button's target for a page, or "" when
// GONE_AUTH_SIGNIN_URL is unset.
//
// Parameters:
//   - page: the page's own path, "/" or "/request"; never request input.
//
// Returns the URL with {return} replaced by the escaped path.
func (g *Gate) SignInURL(page string) string {
	if g == nil {
		return ""
	}
	return strings.ReplaceAll(g.signInURL, returnMarker, url.QueryEscape(page))
}

// ok checks a plain header value: the peer must be a trusted proxy and the
// value must match the format.
//
// Parameters:
//   - remoteAddr: the TCP peer.
//   - v: the trimmed header value.
//
// Returns true when the value is accepted.
func (p *plainCheck) ok(remoteAddr, v string) bool {
	if v == "" || !ratelimit.PeerTrusted(remoteAddr, p.trusted) {
		return false
	}
	switch p.format {
	case "email":
		addr, err := mail.ParseAddress(v)
		return err == nil && addr.Address == v
	case "exact":
		return subtle.ConstantTimeCompare([]byte(v), p.exact) == 1
	}
	return true
}

// ok verifies a signed token: pinned algorithm, known key, valid
// signature, issuer, audience, expiry and a subject.
//
// Parameters:
//   - ctx: request context, for a key refetch.
//   - raw: the compact JWT.
//
// Returns true when the token is valid.
func (c *tokenCheck) ok(ctx context.Context, raw string) bool {
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{c.alg})
	if err != nil || len(tok.Headers) != 1 {
		return false
	}
	key := c.keys.lookup(ctx, tok.Headers[0].KeyID, c.alg)
	if key == nil {
		return false
	}
	var claims jwt.Claims
	if tok.Claims(key, &claims) != nil || claims.Expiry == nil || claims.Subject == "" {
		return false
	}
	want := jwt.Expected{Issuer: c.issuer, AnyAudience: jwt.Audience{c.audience}, Time: c.now()}
	return claims.ValidateWithLeeway(want, leeway) == nil
}

// stripBearer removes a leading "Bearer " (any case) from an Authorization
// value.
//
// Parameters:
//   - v: the header value.
//
// Returns the token.
func stripBearer(v string) string {
	const scheme = "bearer "
	if len(v) > len(scheme) && strings.EqualFold(v[:len(scheme)], scheme) {
		return strings.TrimSpace(v[len(scheme):])
	}
	return v
}
