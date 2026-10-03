package envelope

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"

	"github.com/haukened/gone/internal/domain"
)

// maxFragmentLen bounds fragment parsing work; v1 and v2 fragments are 46
// bytes.
const maxFragmentLen = 512

// Fragment is a validated link fragment: a supported protocol version and
// its key. Construct with NewFragment or ParseFragment.
type Fragment struct {
	version uint8
	key     []byte
}

// NewFragment validates and builds a fragment. The key is copied.
//
// Parameters:
//   - version: protocol version; must be supported.
//   - key: raw link key; exactly KeySize bytes for v1 and v2.
//
// Returns the fragment, domain.ErrInvalidVersion, or ErrInvalidKey.
func NewFragment(version uint8, key []byte) (Fragment, error) {
	if !domain.Supported(version) {
		return Fragment{}, domain.ErrInvalidVersion
	}
	if len(key) != domain.KeySize {
		return Fragment{}, ErrInvalidKey
	}
	return Fragment{version: version, key: bytes.Clone(key)}, nil
}

// Version returns the fragment's protocol version.
//
// Returns the version.
func (f Fragment) Version() uint8 { return f.version }

// Key returns a copy of the fragment's key.
//
// Returns the key bytes.
func (f Fragment) Key() []byte { return bytes.Clone(f.key) }

// String renders the fragment without the leading '#', e.g. "v1:<43 chars>".
//
// Returns the canonical fragment text.
func (f Fragment) String() string {
	return "v" + strconv.Itoa(int(f.version)) + ":" + domain.EncodeB64URL(f.key)
}

// ParseFragment parses a raw, still percent-encoded fragment without its
// leading '#' (docs/protocol.md §7).
//
// Parameters:
//   - s: fragment text, e.g. "v1:<43 chars>".
//
// Returns the fragment; domain.ErrInvalidVersion for a well-formed but
// unsupported version; or ErrInvalidFragment for anything else.
func ParseFragment(s string) (Fragment, error) {
	verStr, payload, ok := splitFragment(s)
	if !ok {
		return Fragment{}, ErrInvalidFragment
	}
	v, err := strconv.ParseUint(verStr, 10, 8)
	if err != nil {
		return Fragment{}, ErrInvalidFragment
	}
	version := uint8(v) // #nosec G115 -- ParseUint bitSize 8 bounds v to 255
	if !domain.Supported(version) {
		return Fragment{}, domain.ErrInvalidVersion
	}
	key, err := domain.DecodeB64URL(payload)
	if err != nil {
		return Fragment{}, ErrInvalidFragment
	}
	f, err := NewFragment(version, key)
	if err != nil {
		return Fragment{}, ErrInvalidFragment
	}
	return f, nil
}

// splitFragment checks the fragment grammar: "v", 1-3 digit canonical
// decimal, ":", and a non-empty base64url-alphabet payload.
//
// Parameters:
//   - s: fragment text without '#'.
//
// Returns the version digits, the payload, and whether the grammar matched.
func splitFragment(s string) (string, string, bool) {
	if len(s) > maxFragmentLen || !strings.HasPrefix(s, "v") {
		return "", "", false
	}
	verStr, payload, found := strings.Cut(s[1:], ":")
	if !found || !isCanonicalDecimal(verStr) || payload == "" || !isAlphabet(payload) {
		return "", "", false
	}
	return verStr, payload, true
}

// isCanonicalDecimal reports whether s is 1-3 ASCII digits without a
// leading zero.
//
// Parameters:
//   - s: candidate string.
//
// Returns true for canonical decimal.
func isCanonicalDecimal(s string) bool {
	if len(s) == 0 || len(s) > 3 || s[0] == '0' {
		return false
	}
	return strings.Trim(s, "0123456789") == ""
}

// isAlphabet reports whether s uses only base64url characters.
//
// Parameters:
//   - s: candidate string.
//
// Returns true if every byte is in [A-Za-z0-9_-].
func isAlphabet(s string) bool {
	return strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_") == ""
}

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
	return Link{Origin: u.Scheme + "://" + u.Host, ID: id, Fragment: f}, nil
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
	u, err := url.Parse(raw)
	if err != nil || !isOrigin(u) || u.RawQuery != "" || u.ForceQuery {
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
	return Link{Origin: u.Scheme + "://" + u.Host, ID: id, Fragment: f}, nil
}

// isOrigin reports whether u is an absolute http(s) URL with a host and no
// user info.
//
// Parameters:
//   - u: parsed URL.
//
// Returns true when u can serve as a link origin.
func isOrigin(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
