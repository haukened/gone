package client

import (
	"net"
	"net/url"
	"strings"
)

// NormalizeOrigin validates and canonicalizes a server origin. A single
// trailing slash is accepted.
//
// Parameters:
//   - raw: origin string.
//   - allowHTTP: whether "http" is acceptable for a loopback host.
//
// Returns the origin as scheme://host, ErrInvalidOrigin, or
// ErrInsecureOrigin.
func NormalizeOrigin(raw string, allowHTTP bool) (string, error) {
	u, err := parseOrigin(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "http" && (!allowHTTP || !loopback(u.Hostname())) {
		return "", ErrInsecureOrigin
	}
	return u.Scheme + "://" + u.Host, nil
}

// CanonicalOrigin validates and canonicalizes an http or https origin
// without deciding whether http may be used; New makes that decision.
//
// Parameters:
//   - raw: origin string.
//
// Returns the origin as scheme://host or ErrInvalidOrigin.
func CanonicalOrigin(raw string) (string, error) {
	u, err := parseOrigin(raw)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// parseOrigin parses a bare http or https origin, lowercasing the scheme
// and host and dropping a default port (":443" for https, ":80" for http)
// or an empty one, so equal origins compare equal. A single trailing slash
// is accepted.
//
// Parameters:
//   - raw: origin string.
//
// Returns the parsed URL or ErrInvalidOrigin.
func parseOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || !bareOrigin(u) {
		return nil, ErrInvalidOrigin
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, ErrInvalidOrigin
	}
	u.Host = strings.ToLower(u.Host)
	if p := u.Port(); p == "" || p == defaultPort[u.Scheme] {
		u.Host = strings.TrimSuffix(u.Host, ":"+p)
	}
	return u, nil
}

// defaultPort maps a scheme to the port a URL may omit.
var defaultPort = map[string]string{"https": "443", "http": "80"}

// bareOrigin reports whether u has a host and nothing beyond scheme and
// host.
//
// Parameters:
//   - u: parsed URL.
//
// Returns true when u has no userinfo, opaque part, path, query or
// fragment.
func bareOrigin(u *url.URL) bool {
	return u.Host != "" && u.User == nil && u.Opaque == "" && u.Path == "" &&
		u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// loopback reports whether host certainly names this machine: exactly
// "localhost", or a literal loopback IP address. Other names, including
// *.localhost, are resolved through DNS like any host, so plain HTTP to
// them could leave the machine; they are not loopback here.
//
// Parameters:
//   - host: hostname without port or brackets.
//
// Returns true for a loopback host.
func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
