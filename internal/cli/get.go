package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// getOpts holds the parsed "gone get" flags.
type getOpts struct {
	common
	out  string
	raw  bool
	pass passOpts
}

// ackError reports that a secret was delivered but the server did not
// confirm its deletion.
type ackError struct{ err error }

// Error returns the user-facing description.
//
// Returns the message.
func (e *ackError) Error() string {
	return "the secret was retrieved, but the server did not confirm its deletion: " + e.err.Error()
}

// Unwrap returns the underlying client error.
//
// Returns the wrapped error.
func (e *ackError) Unwrap() error { return e.err }

// parseGet parses "gone get" arguments.
//
// Parameters:
//   - args: arguments after "get".
//
// Returns the options, the raw link, flag.ErrHelp, or a usage error.
func parseGet(args []string) (getOpts, string, error) {
	var o getOpts
	fs := newFlagSet("get")
	o.register(fs)
	o.pass.register(fs, false)
	for _, name := range []string{"o", "out"} {
		fs.StringVar(&o.out, name, ".", "")
	}
	fs.BoolVar(&o.raw, "raw", false, "")
	raw, err := onePositional(fs, args, "link")
	if err != nil {
		return o, "", err
	}
	return o, raw, validateCommon(o.common)
}

// runGet implements "gone get <link>": validate everything that can be
// checked locally, claim, decrypt, save, print, then acknowledge.
//
// Parameters:
//   - ctx: request context.
//   - args: arguments after "get".
//
// Returns nil or a classified error.
func (a *app) runGet(ctx context.Context, args []string) error {
	o, raw, err := parseGet(args)
	if err != nil {
		return err
	}
	a.json = o.json
	link, err := envelope.ParseLink(raw)
	if err != nil {
		return usagef("not a valid gone link (quote it in single quotes so the shell keeps the #fragment)")
	}
	cl, err := newClient(a.env, link.Origin, "the link", o.common)
	if err != nil {
		return err
	}
	pass, err := a.recipientPassphrase(link.Fragment.Version(), o.pass.file)
	defer clear(pass)
	if err != nil {
		return err
	}
	root, err := openOutDir(o.out)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return a.retrieve(ctx, cl, link, pass, root, o)
}

// retrieve claims, decrypts, delivers and acknowledges one secret.
//
// Parameters:
//   - ctx: request context.
//   - cl: API client for the link's origin.
//   - link: parsed share link.
//   - pass: recipient passphrase; nil for protocol v1.
//   - root: output directory root.
//   - o: get options.
//
// Returns nil or a classified error.
func (a *app) retrieve(ctx context.Context, cl *client.Client, link envelope.Link, pass []byte, root *os.Root, o getOpts) error {
	version := link.Fragment.Version()
	claimed, err := cl.Claim(ctx, link.ID, version)
	if err != nil {
		return err
	}
	defer clear(claimed.Body)
	key := link.Fragment.Key()
	defer clear(key)
	plain, err := a.decrypt(version, key, pass, claimed)
	defer clear(plain)
	if err != nil {
		return err
	}
	if err := a.deliver(root, o, plain); err != nil {
		return err
	}
	if err := cl.Ack(ctx, link.ID, claimed.Token); err != nil {
		return &ackError{err: err}
	}
	return nil
}

// deliver decodes the plaintext, saves attachments and prints the result.
//
// Parameters:
//   - root: output directory root.
//   - o: get options.
//   - plain: decrypted plaintext.
//
// Returns nil, an integrity error, or an I/O error.
func (a *app) deliver(root *os.Root, o getOpts, plain []byte) error {
	p, err := envelope.Unpack(plain)
	if err != nil {
		return err
	}
	saved, err := saveFiles(root, o.out, p.Files)
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			Message string      `json:"message"`
			Files   []savedFile `json:"files"`
		}{string(p.Message), saved})
	}
	for _, s := range saved {
		_, _ = fmt.Fprintf(a.env.Stderr, "Saved: %s (%d bytes)\n", escapeTerminal(s.Path), s.Size)
	}
	return a.printMessage(p.Message, o.raw)
}

// printMessage writes the message to stdout. On a terminal, control
// characters are escaped (unless raw) and a final newline is ensured.
//
// Parameters:
//   - msg: message bytes.
//   - raw: whether to skip escaping.
//
// Returns nil or an I/O error.
func (a *app) printMessage(msg []byte, raw bool) error {
	if !a.env.StdoutTTY {
		return writeBytes(a.env.Stdout, msg)
	}
	s := string(msg)
	if !raw {
		s = escapeTerminal(s)
	}
	if len(s) > 0 && s[len(s)-1] != '\n' {
		s += "\n"
	}
	return writeText(a.env.Stdout, s)
}
