package envelope

import (
	"bytes"
	"strings"

	"github.com/haukened/gone/v3/internal/domain"
)

// replyFragmentPrefix starts every v3 reply-link fragment.
const replyFragmentPrefix = "v3:"

// ReplyFragment is a validated reply-link fragment (protocol §7.4): the
// requester's public key and the request's fill token.
type ReplyFragment struct {
	// PublicKey is the requester's uncompressed P-256 public key (65 bytes,
	// first byte 0x04). Whether it is a point on the curve is checked when
	// sealing.
	PublicKey []byte
	// Fill authorizes the reply; send it only in X-Gone-Fill.
	Fill domain.FillToken
}

// String renders the fragment without the leading '#':
// "v3:" b64u(pubR) "." fillToken.
//
// Returns the canonical fragment text.
func (f ReplyFragment) String() string {
	return replyFragmentPrefix + domain.EncodeB64URL(f.PublicKey) + "." + f.Fill.String()
}

// ParseReplyFragment parses a raw, still percent-encoded reply fragment
// without its leading '#'. It checks the grammar and lengths only.
//
// Parameters:
//   - s: fragment text, e.g. "v3:<87 chars>.<43 chars>".
//
// Returns the fragment; domain.ErrInvalidVersion for a well-formed fragment
// of another version; or ErrInvalidFragment for anything else.
func ParseReplyFragment(s string) (ReplyFragment, error) {
	pubStr, fillStr, err := splitReplyFragment(s)
	if err != nil {
		return ReplyFragment{}, err
	}
	pub, err := domain.DecodeB64URL(pubStr)
	if err != nil || len(pub) != domain.V3PublicKeySize || pub[0] != 0x04 {
		return ReplyFragment{}, ErrInvalidFragment
	}
	fill, err := domain.ParseFillToken(fillStr)
	if err != nil {
		return ReplyFragment{}, ErrInvalidFragment
	}
	return ReplyFragment{PublicKey: pub, Fill: fill}, nil
}

// splitReplyFragment checks the reply fragment grammar: "v", canonical
// decimal 3, ":", a base64url public key, ".", and a base64url fill token.
//
// Parameters:
//   - s: fragment text without '#'.
//
// Returns the two payload parts; domain.ErrInvalidVersion for another
// canonical version; or ErrInvalidFragment.
func splitReplyFragment(s string) (string, string, error) {
	payload, err := replyPayload(s)
	if err != nil {
		return "", "", err
	}
	pubStr, fillStr, found := strings.Cut(payload, ".")
	if !found || pubStr == "" || !isAlphabet(pubStr) || !isAlphabet(fillStr) {
		return "", "", ErrInvalidFragment
	}
	return pubStr, fillStr, nil
}

// replyPayload checks the "v" version ":" prefix of a reply fragment.
//
// Parameters:
//   - s: fragment text without '#'.
//
// Returns the text after the prefix; domain.ErrInvalidVersion for another
// canonical version; or ErrInvalidFragment.
func replyPayload(s string) (string, error) {
	if len(s) > maxFragmentLen || !strings.HasPrefix(s, "v") {
		return "", ErrInvalidFragment
	}
	verStr, payload, found := strings.Cut(s[1:], ":")
	if !found || !isCanonicalDecimal(verStr) {
		return "", ErrInvalidFragment
	}
	if verStr != "3" {
		return "", domain.ErrInvalidVersion
	}
	return payload, nil
}

// ReplyLink is a parsed reply link: the link a requester gives to the person
// who holds the secret.
type ReplyLink struct {
	// Origin is scheme://host[:port], with no path, user info, query or fragment.
	Origin string
	// ID is the request ID.
	ID domain.SecretID
	// Fragment carries the requester's public key and the fill token.
	Fragment ReplyFragment
}

// String renders the link as <origin>/reply/<id>#<fragment>.
//
// Returns the link text.
func (l ReplyLink) String() string {
	return l.Origin + "/reply/" + l.ID.String() + "#" + l.Fragment.String()
}

// NewReplyFragment validates and builds a reply fragment. The key is copied.
//
// Parameters:
//   - pub: the requester's uncompressed P-256 public key.
//   - fill: the fill token returned by the server.
//
// Returns the fragment or ErrInvalidFragment.
func NewReplyFragment(pub []byte, fill string) (ReplyFragment, error) {
	return ParseReplyFragment(replyFragmentPrefix + domain.EncodeB64URL(bytes.Clone(pub)) + "." + fill)
}

// ParseReplyLink parses an absolute reply link. The path must be exactly
// /reply/<id>; the fragment is matched raw (still percent-encoded).
//
// Parameters:
//   - raw: the full link.
//
// Returns the link; domain.ErrInvalidVersion for another fragment version; or
// ErrInvalidLink / ErrInvalidFragment.
func ParseReplyLink(raw string) (ReplyLink, error) {
	u, err := parseSecretLinkURL(raw)
	if err != nil {
		return ReplyLink{}, ErrInvalidLink
	}
	idStr, ok := strings.CutPrefix(u.EscapedPath(), "/reply/")
	id, idErr := domain.ParseID(idStr)
	if !ok || idErr != nil {
		return ReplyLink{}, ErrInvalidLink
	}
	f, err := ParseReplyFragment(u.EscapedFragment())
	if err != nil {
		return ReplyLink{}, err
	}
	return ReplyLink{Origin: u.Scheme + "://" + u.Host, ID: id, Fragment: f}, nil
}
