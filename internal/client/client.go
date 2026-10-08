// Package client is a minimal HTTP client for the Gone API
// (docs/protocol.md §8). It never sees plaintext or keys: callers seal and
// open ciphertext with the envelope package and pass only opaque bytes and
// protocol metadata. Every response is size-bounded, redirects are never
// followed, and server error bodies are never surfaced to the caller.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds one whole request, including the body transfer.
	DefaultTimeout = 60 * time.Second
	// MaxCiphertext caps a claimed ciphertext body.
	MaxCiphertext = 64 << 20
	// maxJSONBody caps a JSON success body.
	maxJSONBody = 4 << 10
	// maxErrorBody caps how much of an error body is drained before closing.
	maxErrorBody = 4 << 10
)

// Options configures a Client.
type Options struct {
	// AllowHTTP permits a plain-HTTP origin on a loopback host, the same
	// exception browsers make for localhost. Any other http origin is always
	// refused. TLS certificates are always verified when HTTPS is used.
	AllowHTTP bool
	// Timeout bounds each request; zero means DefaultTimeout.
	Timeout time.Duration
	// UserAgent is sent on every request when non-empty.
	UserAgent string
	// RootCAs overrides the system trust store; nil uses the system pool.
	RootCAs *x509.CertPool
}

// Client talks to one Gone server origin.
type Client struct {
	origin string
	hc     *http.Client
	ua     string
}

// New returns a Client for origin, which must be a bare scheme://host[:port]
// URL with no user info, path, query, or fragment.
//
// Parameters:
//   - origin: server origin, such as "https://gone.example".
//   - opts: transport options.
//
// Returns the client, ErrInvalidOrigin, or ErrInsecureOrigin.
func New(origin string, opts Options) (*Client, error) {
	o, err := NormalizeOrigin(origin, opts.AllowHTTP)
	if err != nil {
		return nil, err
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: opts.RootCAs},
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	hc := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		// Never follow redirects: they could forward bearer headers to
		// another host. A 3xx surfaces as ErrServer.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Client{origin: o, hc: hc, ua: opts.UserAgent}, nil
}

// Origin returns the normalized server origin.
//
// Returns the origin as scheme://host without a trailing slash.
func (c *Client) Origin() string { return c.origin }

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
// and host. A single trailing slash is accepted.
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
	return u, nil
}

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

// loopback reports whether host names this machine: localhost, a
// *.localhost name, or a loopback IP address. Browsers treat these as
// secure contexts too.
//
// Parameters:
//   - host: hostname without port or brackets.
//
// Returns true for a loopback host.
func loopback(host string) bool {
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// do sends one request to path on the client's origin.
//
// Parameters:
//   - ctx: request context.
//   - method: HTTP method.
//   - path: absolute path, such as "/api/secret".
//   - hdr: extra request headers; may be nil.
//   - body: request body bytes; nil for no body.
//
// Returns the response, or an error wrapping ErrNetwork whose message names
// the origin but not the request path.
func (c *Client) do(ctx context.Context, method, path string, hdr http.Header, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, rd)
	if err != nil {
		return nil, wrapNetwork(err)
	}
	for k, vs := range hdr {
		req.Header[k] = vs
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	resp, err := c.hc.Do(req) // #nosec G704 -- origin is validated by NormalizeOrigin
	if err != nil {
		// The request path holds the secret ID, which is enough to burn
		// the secret; report only the origin.
		var ue *url.Error
		if errors.As(err, &ue) {
			ue.URL = c.origin
		}
		return nil, wrapNetwork(err)
	}
	return resp, nil
}

// expect checks resp's status code. On a mismatch it drains a bounded part of
// the body, closes it, and returns the mapped error.
//
// Parameters:
//   - resp: response to check.
//   - want: expected status code.
//
// Returns nil when the status matches, or a mapped error.
func expect(resp *http.Response, want int) error {
	if resp.StatusCode == want {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	_ = resp.Body.Close()
	return statusError(resp)
}

// discard drains a bounded part of resp's body and closes it.
//
// Parameters:
//   - resp: response to release.
func discard(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	_ = resp.Body.Close()
}

// readBounded reads at most limit bytes from resp and closes the body.
//
// Parameters:
//   - resp: response whose body is read.
//   - limit: maximum body size in bytes.
//
// Returns the body, ErrTooLarge, ErrIntegrity on a Content-Length mismatch,
// or an error wrapping ErrNetwork.
func readBounded(resp *http.Response, limit int64) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.ContentLength > limit {
		return nil, ErrTooLarge
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		clear(b)
		return nil, wrapNetwork(err)
	}
	if int64(len(b)) > limit {
		clear(b)
		return nil, ErrTooLarge
	}
	if resp.ContentLength >= 0 && int64(len(b)) != resp.ContentLength {
		clear(b)
		return nil, ErrIntegrity
	}
	return b, nil
}

// single returns the only value of header name, or false when it is absent
// or repeated.
//
// Parameters:
//   - h: response headers.
//   - name: header name.
//
// Returns the value and whether exactly one was present.
func single(h http.Header, name string) (string, bool) {
	vs := h.Values(name)
	if len(vs) != 1 {
		return "", false
	}
	return vs[0], true
}
