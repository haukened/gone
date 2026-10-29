package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// msgSignInRequired explains why the CLI can't create on a sign-in server.
const msgSignInRequired = "this server requires signing in through its web page; the gone CLI can't create secrets or requests there yet"

// msgPolicyNeedsPhrase explains a server's passphrase policy and the fix.
const msgPolicyNeedsPhrase = "this server requires a passphrase; add --passphrase-generate, --passphrase-prompt or --passphrase-file"

// serverPolicy fetches the operator policy of cl's server. A server older
// than v3.12 has no policy route, which means no passphrase rule and the
// envelope's own file limit.
//
// Parameters:
//   - ctx: request context.
//   - cl: client for the server.
//
// Returns the policy or a client error.
func serverPolicy(ctx context.Context, cl *client.Client) (client.Policy, error) {
	p, err := cl.Policy(ctx)
	if errors.Is(err, client.ErrNotFound) {
		return client.Policy{MaxFiles: envelope.MaxFiles}, nil
	}
	return p, err
}

// policyErrorf builds a usage error of kind "policy": the input is valid,
// but this server's operator does not allow it.
//
// Parameters:
//   - format: fmt format string.
//   - args: format arguments.
//
// Returns the error.
func policyErrorf(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...), kind: "policy"}
}

// checkFiles reports whether n attachments fit the server's file limit.
//
// Parameters:
//   - n: number of --file flags.
//   - p: server policy.
//
// Returns nil or a policy error.
func checkFiles(n int, p client.Policy) error {
	switch {
	case n <= p.MaxFiles:
		return nil
	case p.MaxFiles == 0:
		return policyErrorf("this server does not accept files")
	default:
		return policyErrorf("too many files: this server accepts at most %d", p.MaxFiles)
	}
}

// checkSendPolicy checks "gone send" options against the server's policy
// before anything is read, prompted for or encrypted: sign-in first, then
// the passphrase rule, then the file limit.
//
// Parameters:
//   - o: parsed send options.
//   - p: server policy.
//
// Returns nil or a policy error.
func checkSendPolicy(o sendOpts, p client.Policy) error {
	if p.SignInRequired {
		return policyErrorf(msgSignInRequired)
	}
	if p.PassphraseRequired && !o.pass.set() {
		return policyErrorf(msgPolicyNeedsPhrase)
	}
	return checkFiles(len(o.in.files), p)
}

// checkRequestPolicy refuses "gone request" on a server that requires
// sign-in, before a key pair is made or anything is saved.
//
// Parameters:
//   - ctx: request context.
//   - cl: client for the server.
//
// Returns nil, a policy error, or a client error.
func checkRequestPolicy(ctx context.Context, cl *client.Client) error {
	p, err := serverPolicy(ctx, cl)
	if err != nil {
		return err
	}
	if p.SignInRequired {
		return policyErrorf(msgSignInRequired)
	}
	return nil
}
