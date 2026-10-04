package client

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Sentinel errors. Server error bodies are never included in messages.
var (
	// ErrInvalidOrigin is returned for a malformed server origin.
	ErrInvalidOrigin = errors.New("invalid server origin")
	// ErrInsecureOrigin is returned for an http origin without AllowHTTP.
	ErrInsecureOrigin = errors.New("refusing plain-HTTP server origin")
	// ErrNotFound is the server's uniform 404: missing, opened, expired,
	// revoked, or a wrong token.
	ErrNotFound = errors.New("not found")
	// ErrRateLimited is returned for HTTP 429; see RateLimitError.
	ErrRateLimited = errors.New("rate limited")
	// ErrTooLarge is returned for HTTP 413 or an oversized response.
	ErrTooLarge = errors.New("too large")
	// ErrRejected is returned for any other 4xx response.
	ErrRejected = errors.New("request rejected by server")
	// ErrServer is returned for 5xx and any unexpected status, including 3xx.
	ErrServer = errors.New("server error")
	// ErrProtocol is returned for a malformed success response.
	ErrProtocol = errors.New("malformed server response")
	// ErrVersionMismatch is returned when a claimed secret's version differs
	// from the link's.
	ErrVersionMismatch = errors.New("unsupported protocol version")
	// ErrIntegrity is returned for a malformed nonce or a truncated body. It
	// is reported like a decryption failure (docs/protocol.md §8.2).
	ErrIntegrity = errors.New("integrity check failed")
	// ErrNetwork wraps transport failures.
	ErrNetwork = errors.New("network error")
)

// StatusError records an unexpected HTTP status. It unwraps to one of the
// status sentinels above.
type StatusError struct {
	// Code is the HTTP status code.
	Code int
	kind error
}

// Error returns a message naming the status code.
//
// Returns the formatted message.
func (e *StatusError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", e.kind, e.Code)
}

// Unwrap returns the sentinel this status maps to.
//
// Returns the sentinel error.
func (e *StatusError) Unwrap() error { return e.kind }

// RateLimitError is returned for HTTP 429.
type RateLimitError struct {
	// RetryAfter is the server's Retry-After hint; zero when absent.
	RetryAfter time.Duration
}

// Error returns a message including the retry hint when known.
//
// Returns the formatted message.
func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited; retry after %s", e.RetryAfter)
	}
	return "rate limited"
}

// Unwrap returns ErrRateLimited.
//
// Returns ErrRateLimited.
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// maxRetryAfter caps a Retry-After hint so a hostile server cannot make the
// CLI report an absurd delay.
const maxRetryAfter = 24 * time.Hour

// statusError maps an unexpected response status to an error.
//
// Parameters:
//   - resp: response with an unexpected status.
//
// Returns *RateLimitError for 429, otherwise *StatusError.
func statusError(resp *http.Response) error {
	code := resp.StatusCode
	switch {
	case code == http.StatusTooManyRequests:
		return &RateLimitError{RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	case code == http.StatusNotFound:
		return &StatusError{Code: code, kind: ErrNotFound}
	case code == http.StatusRequestEntityTooLarge:
		return &StatusError{Code: code, kind: ErrTooLarge}
	case code >= 400 && code < 500:
		return &StatusError{Code: code, kind: ErrRejected}
	default:
		return &StatusError{Code: code, kind: ErrServer}
	}
}

// retryAfter parses a Retry-After value in whole seconds. HTTP-date values
// and anything malformed yield zero.
//
// Parameters:
//   - v: header value.
//
// Returns the delay, capped at maxRetryAfter.
func retryAfter(v string) time.Duration {
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0
	}
	d := time.Duration(n) * time.Second
	return min(d, maxRetryAfter)
}

// wrapNetwork wraps a transport error with ErrNetwork.
//
// Parameters:
//   - err: transport error.
//
// Returns an error matching both ErrNetwork and err.
func wrapNetwork(err error) error {
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}
