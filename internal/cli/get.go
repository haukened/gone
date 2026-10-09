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
	out        string
	messageOut string
	raw        bool
	pass       passOpts
}

// outputs are the opened destinations for one "gone get".
type outputs struct {
	// root is the attachment directory.
	root *os.Root
	// msg is the --message-out destination, or nil to print the message.
	msg *messageOut
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
	fs.StringVar(&o.messageOut, "message-out", "", "")
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
	if raw, err = a.readLinkArg(raw); err != nil {
		return err
	}
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
	out, err := openOutputs(o)
	if err != nil {
		return err
	}
	defer out.close()
	return a.retrieve(ctx, cl, link, pass, out, o)
}

// openOutputs opens and checks the attachment directory and, when set, the
// --message-out destination, before the claim.
//
// Parameters:
//   - o: get options.
//
// Returns the opened outputs or a usage/I/O error.
func openOutputs(o getOpts) (outputs, error) {
	root, err := openOutDir(o.out)
	if err != nil {
		return outputs{}, err
	}
	out := outputs{root: root}
	if o.messageOut != "" {
		if out.msg, err = openMessageOut(o.messageOut); err != nil {
			out.close()
			return outputs{}, err
		}
	}
	return out, nil
}

// close releases the opened destinations.
func (out outputs) close() {
	if out.root != nil {
		_ = out.root.Close()
	}
	out.msg.close()
}

// retrieve claims, decrypts, delivers and acknowledges one secret.
//
// Parameters:
//   - ctx: request context.
//   - cl: API client for the link's origin.
//   - link: parsed share link.
//   - pass: recipient passphrase; nil for protocol v1.
//   - out: opened output destinations.
//   - o: get options.
//
// Returns nil or a classified error.
func (a *app) retrieve(ctx context.Context, cl *client.Client, link envelope.Link, pass []byte, out outputs, o getOpts) error {
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
	if err := a.deliver(out, o, plain); err != nil {
		return err
	}
	if err := cl.Ack(ctx, link.ID, claimed.Token); err != nil {
		return &ackError{err: err}
	}
	return nil
}

// deliver decodes the plaintext, writes the message file (if any) and the
// attachments, and reports the result. If an attachment cannot be saved,
// the message file is removed again.
//
// Parameters:
//   - out: opened output destinations.
//   - o: get options.
//   - plain: decrypted plaintext.
//
// Returns nil, an integrity error, or an I/O error.
func (a *app) deliver(out outputs, o getOpts, plain []byte) error {
	p, err := envelope.Unpack(plain)
	if err != nil {
		return err
	}
	if out.msg != nil {
		if err := out.msg.write(p.Message); err != nil {
			return err
		}
	}
	saved, err := saveFiles(out.root, o.out, p.Files)
	if err != nil {
		out.msg.remove()
		return err
	}
	if out.msg != nil {
		return a.reportMessageFile(out.msg.path, len(p.Message), saved)
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			Message string      `json:"message"`
			Files   []savedFile `json:"files"`
		}{string(p.Message), saved})
	}
	a.reportSaved(saved)
	return a.printMessage(p.Message, o.raw)
}

// messageFile describes the message written by --message-out.
type messageFile struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

// reportMessageFile reports a message saved with --message-out. The
// plaintext is never written to stdout or stderr.
//
// Parameters:
//   - path: message file path as given.
//   - size: message length in bytes.
//   - saved: saved attachments.
//
// Returns an output error, if any.
func (a *app) reportMessageFile(path string, size int, saved []savedFile) error {
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			MessageFile messageFile `json:"message_file"`
			Files       []savedFile `json:"files"`
		}{messageFile{path, size}, saved})
	}
	a.reportSaved(saved)
	_, _ = fmt.Fprintf(a.env.Stderr, "Saved message: %s (%d bytes)\n", escapeTerminal(path), size)
	return nil
}

// reportSaved lists saved attachments on stderr.
//
// Parameters:
//   - saved: saved attachments.
func (a *app) reportSaved(saved []savedFile) {
	for _, s := range saved {
		_, _ = fmt.Fprintf(a.env.Stderr, "Saved: %s (%d bytes)\n", escapeTerminal(s.Path), s.Size)
	}
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
