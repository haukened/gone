package cli

import (
	"errors"
	"net/url"
	"strings"
	"unicode"

	"github.com/haukened/gone/v3/internal/domain"
)

// linkKind is the kind of link a command takes.
type linkKind int

const (
	secretLinkKind  linkKind = iota // /secret/<id>#v1:… for get
	manageLinkKind                  // /manage/<id>#… for status and revoke
	requestLinkKind                 // /reply/<id>#v3:… for reply
)

// linkKinds maps each kind to its path prefix, its name in errors, what it
// points to, and the command that takes it.
var linkKinds = map[linkKind]struct{ path, noun, target, command string }{
	secretLinkKind:  {"/secret/", "gone link", "a Gone secret", "gone get"},
	manageLinkKind:  {"/manage/", "manage link", "a Gone secret's manage page", "gone status or gone revoke"},
	requestLinkKind: {"/reply/", "gone request link", "a Gone request", "gone reply"},
}

// Characters that come along when a link is copied but are never part of
// one: quotes from cmd.exe (which passes them through) or smart quotes,
// brackets around a link in an email, and punctuation that ends a sentence.
// A gone link starts with "h" and ends in base64url, so trimming these
// never changes a valid link.
const (
	linkLeadJunk  = "'\"‘’“”<([{"
	linkTrailJunk = "'\"‘’“”>)]}.,;:!?"
)

// normalizeLink undoes what shells, editors and mail clients do to a
// pasted link, removing only characters a gone link can't contain:
//   - whitespace anywhere (a link wrapped across lines, a stray newline),
//     including no-break spaces, and invisible format characters such as
//     zero-width spaces and a byte-order mark;
//   - quotes, brackets and sentence punctuation around it;
//   - the backslashes zsh adds when a URL is pasted, which survive inside
//     quotes (see unescapePastedLink).
//
// Parameters:
//   - s: the link as given.
//
// Returns the cleaned link, for the parser to judge.
func normalizeLink(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimLeft(s, linkLeadJunk)
	s = strings.TrimRight(s, linkTrailJunk)
	return unescapePastedLink(s)
}

// linkPunct holds the ASCII punctuation a shell may escape in a pasted
// link.
const linkPunct = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// unescapePastedLink undoes the backslashes zsh adds when a URL is pasted
// (url-quote-magic and bracketed-paste-url-magic turn "#" into "\#").
// Unquoted, the shell removes them again; inside quotes they reach gone as
// part of the link. A gone link never contains a backslash, so one before
// ASCII punctuation is dropped and the rest is left for the parser to
// judge.
//
// Parameters:
//   - s: the link as given.
//
// Returns the link without paste escapes.
func unescapePastedLink(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && strings.IndexByte(linkPunct, s[i+1]) >= 0 {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// diagnoseLink says why a link didn't parse and what to do about it. It
// describes the link but never repeats any of it, so the message can't
// leak a key or ID.
//
// Parameters:
//   - s: the normalized link.
//   - want: the kind of link the command takes.
//   - err: the parser's error.
//
// Returns a usage message.
func diagnoseLink(s string, want linkKind, err error) string {
	lead := "not a valid " + linkKinds[want].noun + ": "
	if why := linkProblem(s, want, err); why != "" {
		return lead + why
	}
	return lead + "it's incomplete or damaged." + retryLink
}

// retryLink ends a diagnosis whose fix is copying the link again.
const retryLink = " Copy it again, or pass - and paste it."

// linkProblem finds a specific reason a link is invalid.
//
// Parameters:
//   - s: the normalized link.
//   - want: the kind of link the command takes.
//   - err: the parser's error.
//
// Returns the reason, or "" when none is known.
func linkProblem(s string, want linkKind, err error) string {
	u, perr := url.Parse(s)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "it should start with https://." + retryLink
	}
	if isLinkScanner(u.Hostname()) {
		return "your email rewrote it to go through a link scanner. Open it in a browser, or ask the sender to send the link another way."
	}
	path := u.EscapedPath()
	if strings.Contains(strings.ToLower(path), "%23") {
		return "its # was changed to %23 on the way, so the key may have reached a server as part of the address. Ask the sender for a new link."
	}
	if why := wrongKind(path, want); why != "" {
		return why
	}
	return contentProblem(s, path, want, err)
}

// wrongKind names the command for a link of another kind.
//
// Parameters:
//   - path: the link's escaped path.
//   - want: the kind of link the command takes.
//
// Returns the reason, or "" when the path isn't another kind's.
func wrongKind(path string, want linkKind) string {
	for kind, other := range linkKinds {
		if kind != want && strings.HasPrefix(path, other.path) {
			return "that's a " + other.noun + "; use " + other.command + "."
		}
	}
	return ""
}

// contentProblem explains a link of the right kind that still didn't
// parse.
//
// Parameters:
//   - s: the normalized link.
//   - path: its escaped path.
//   - want: the kind of link the command takes.
//   - err: the parser's error.
//
// Returns the reason, or "" when none is known.
func contentProblem(s, path string, want linkKind, err error) string {
	switch {
	case !strings.HasPrefix(path, linkKinds[want].path):
		return "it isn't a link to " + linkKinds[want].target + "." + retryLink
	case errors.Is(err, domain.ErrInvalidVersion):
		return "it was made by a newer version of Gone. Upgrade gone (see \"gone version\"), or open it in a browser."
	case !strings.Contains(s, "#"):
		return "the key (the part after #) is missing." + retryLink
	}
	return ""
}

// isLinkScanner reports whether host belongs to an email link-scanning
// service that wraps links (Microsoft Safe Links, Proofpoint, Mimecast).
//
// Parameters:
//   - host: the link's host name.
//
// Returns true for a known scanner.
func isLinkScanner(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".safelinks.protection.outlook.com") ||
		strings.HasPrefix(host, "urldefense.") ||
		strings.HasSuffix(host, ".mimecast.com")
}
