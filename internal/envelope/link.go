package envelope

import (
	"github.com/haukened/gone/v3/internal/domain"
	"net/url"
	"strings"
)

// Link is a parsed secret link.
type Link struct {
	// Origin is scheme://host[:port], with no path, user info, query or fragment.
	Origin string
	// ID is the secret ID.
	ID domain.SecretID
	// Fragment carries the protocol version and key.
	Fragment Fragment
}

// String renders the link as <origin>/secret/<id>#<fragment>.
//
// Returns the link text.
func (l Link) String() string {
	return l.Origin + "/secret/" + l.ID.String() + "#" + l.Fragment.String()
}

// NewLink validates the parts and builds a link.
//
// Parameters:
//   - origin: absolute http(s) origin, e.g. "https://gone.example".
//   - id: secret ID returned by the server.
//   - f: fragment from NewFragment.
//
// Returns the link or ErrInvalidLink.
func NewLink(origin string, id domain.SecretID, f Fragment) (Link, error) {
	u, err := url.Parse(origin)
	if err != nil || !isBareOrigin(u) || !id.Valid() || !domain.Supported(f.version) {
		return Link{}, ErrInvalidLink
	}
	return Link{Origin: originOf(u), ID: id, Fragment: f}, nil
}

// isBareOrigin reports whether u is an http(s) origin with nothing after the
// host: no path, query or fragment.
//
// Parameters:
//   - u: parsed URL.
//
// Returns true for a bare origin.
func isBareOrigin(u *url.URL) bool {
	return isOrigin(u) && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// ParseLink parses an absolute secret link. The path must be exactly
// /secret/<id>; the fragment is matched raw (still percent-encoded).
//
// Parameters:
//   - raw: the full link.
//
// Returns the link; domain.ErrInvalidVersion for an unsupported version; or
// ErrInvalidLink / ErrInvalidFragment.
func ParseLink(raw string) (Link, error) {
	u, err := parseSecretLinkURL(raw)
	if err != nil {
		return Link{}, ErrInvalidLink
	}
	idStr, ok := strings.CutPrefix(u.EscapedPath(), "/secret/")
	id, idErr := domain.ParseID(idStr)
	if !ok || idErr != nil {
		return Link{}, ErrInvalidLink
	}
	f, err := ParseFragment(u.EscapedFragment())
	if err != nil {
		return Link{}, err
	}
	return Link{Origin: originOf(u), ID: id, Fragment: f}, nil
}

// parseSecretLinkURL parses and validates the URL-level secret-link shape.
//
// Parameters:
//   - raw: full secret link.
//
// Returns the parsed URL or ErrInvalidLink.
func parseSecretLinkURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || !isOrigin(u) || u.RawQuery != "" || u.ForceQuery {
		return nil, ErrInvalidLink
	}
	return u, nil
}

// originOf renders u's scheme and host as an origin. The host is escaped
// again (url.URL.Host holds it decoded), so a host such as an IPv6 zone
// "[fe80::1%25en0]" keeps its "%25" and the origin parses back to itself.
//
// Parameters:
//   - u: parsed URL.
//
// Returns scheme://host[:port].
func originOf(u *url.URL) string {
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

// isOrigin reports whether u is an absolute http(s) URL with a host and no
// user info, whose origin parses back to the same host. net/url accepts a
// raw non-ASCII byte in an IPv6 zone but refuses it escaped, so such a host
// could be read from a link but never written into one.
//
// Parameters:
//   - u: parsed URL.
//
// Returns true when u can serve as a link origin.
func isOrigin(u *url.URL) bool {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return false
	}
	back, err := url.Parse(originOf(u))
	return err == nil && back.Host == u.Host
}
