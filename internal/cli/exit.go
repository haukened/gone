package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// Process exit codes. They are part of the CLI's public contract
// (docs/cli.md) and must not be renumbered.
const (
	exitOK          = 0
	exitInternal    = 1
	exitUsage       = 2
	exitNotFound    = 3
	exitPassphrase  = 4
	exitRateLimited = 5
	exitIntegrity   = 6
	exitNetwork     = 7
	exitIO          = 8
)

// errWrongPassphrase is returned when every v2 decryption attempt failed.
var errWrongPassphrase = errors.New("wrong passphrase")

// Request messages for a 404: the server can't say which case it was.
const (
	msgRequestGone = "request not found: it expired, was cancelled, or its reply was already opened"
	msgReplyGone   = "request not found: it was already answered or cancelled, or it expired"
)

// notFoundError is an exit-3 failure with its own message, for requests.
type notFoundError struct{ msg string }

// Error returns the message.
func (e *notFoundError) Error() string { return e.msg }

// msgIntegrity is the single message for every decryption or integrity
// failure, so a tampered response cannot be told apart from a bad key.
const msgIntegrity = "the secret could not be decrypted: the link is wrong or the data was altered"

// usageError reports invalid command-line input.
type usageError struct{ msg string }

// Error returns the usage message.
//
// Returns the message.
func (e *usageError) Error() string { return e.msg }

// usagef builds a usage error.
//
// Parameters:
//   - format: fmt format string.
//   - args: format arguments.
//
// Returns a *usageError.
func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// ioError reports a local file or stream failure.
type ioError struct {
	op  string
	err error
}

// Error returns "op: cause".
//
// Returns the message.
func (e *ioError) Error() string { return e.op + ": " + e.err.Error() }

// Unwrap returns the underlying error.
//
// Returns the cause.
func (e *ioError) Unwrap() error { return e.err }

// ioErr wraps err as a local I/O failure.
//
// Parameters:
//   - op: short description of the operation.
//   - err: underlying error.
//
// Returns an *ioError.
func ioErr(op string, err error) error { return &ioError{op: op, err: err} }

// failure is a classified error ready to report.
type failure struct {
	code       int
	kind       string
	msg        string
	retryAfter time.Duration
}

// sentinelClass maps a sentinel error to its exit code, JSON kind and an
// optional fixed message (empty means use err.Error()).
type sentinelClass struct {
	err  error
	code int
	kind string
	msg  string
}

// sentinelClasses is checked in order; the first errors.Is match wins.
var sentinelClasses = []sentinelClass{
	{envelope.ErrInvalidPassphrase, exitUsage, "usage", "the passphrase must be 1 to 1024 bytes of UTF-8"},
	{client.ErrInvalidOrigin, exitUsage, "usage", ""},
	{client.ErrInsecureOrigin, exitUsage, "usage", ""},
	{client.ErrNotFound, exitNotFound, "not_found", "secret not found: it was already opened, revoked or expired, or never existed"},
	{errWrongPassphrase, exitPassphrase, "wrong_passphrase", "wrong passphrase (or the link is damaged)"},
	{client.ErrIntegrity, exitIntegrity, "integrity", msgIntegrity},
	{client.ErrVersionMismatch, exitIntegrity, "integrity", msgIntegrity},
	{envelope.ErrDecrypt, exitIntegrity, "integrity", msgIntegrity},
	{envelope.ErrMalformed, exitIntegrity, "integrity", msgIntegrity},
	{envelope.ErrInvalidEnvelope, exitIntegrity, "integrity", msgIntegrity},
	{envelope.ErrTooManyFiles, exitIntegrity, "integrity", msgIntegrity},
	{envelope.ErrInvalidKey, exitIntegrity, "integrity", "the request link is damaged: its key is not valid"},
	{client.ErrNetwork, exitNetwork, "network", ""},
	{client.ErrTooLarge, exitNetwork, "too_large", "the secret is larger than the server accepts"},
	{client.ErrServer, exitNetwork, "server", ""},
	{client.ErrRejected, exitNetwork, "server", ""},
	{client.ErrProtocol, exitNetwork, "server", ""},
	{context.Canceled, exitInternal, "interrupted", "interrupted"},
}

// classifyRequest classifies the request-specific exit-3 failures.
//
// Parameters:
//   - err: the error.
//
// Returns the failure and true, or false when err is not one of them.
func classifyRequest(err error) (failure, bool) {
	var nf *notFoundError
	if errors.As(err, &nf) {
		return failure{code: exitNotFound, kind: "not_found", msg: nf.msg}, true
	}
	if errors.Is(err, errNoReplyYet) {
		return failure{code: exitNotFound, kind: "not_ready", msg: errNoReplyYet.Error()}, true
	}
	return failure{}, false
}

// classify maps an error to its exit code and report.
//
// Parameters:
//   - err: non-nil command error.
//
// Returns the classified failure.
func classify(err error) failure {
	var ae *ackError
	if errors.As(err, &ae) {
		return failure{code: exitNetwork, kind: "not_confirmed", msg: ae.Error()}
	}
	if f, ok := classifyRequest(err); ok {
		return f
	}
	var ue *usageError
	if errors.As(err, &ue) {
		return failure{code: exitUsage, kind: "usage", msg: ue.msg}
	}
	var rl *client.RateLimitError
	if errors.As(err, &rl) {
		return failure{code: exitRateLimited, kind: "rate_limited", msg: rl.Error(), retryAfter: rl.RetryAfter}
	}
	var ie *ioError
	if errors.As(err, &ie) {
		return failure{code: exitIO, kind: "io", msg: ie.Error()}
	}
	for _, c := range sentinelClasses {
		if errors.Is(err, c.err) {
			return failure{code: c.code, kind: c.kind, msg: cmp(c.msg, err.Error())}
		}
	}
	return failure{code: exitInternal, kind: "internal", msg: err.Error()}
}

// cmp returns a unless it is empty, in which case it returns b.
//
// Parameters:
//   - a: preferred value.
//   - b: fallback value.
//
// Returns a or b.
func cmp(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
