package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// replyOpts holds `gone reply` flags.
type replyOpts struct {
	common
	files       stringList
	messageFile string
}

// parseReply parses `gone reply` flags.
//
// Parameters:
//   - args: arguments after "reply".
//
// Returns the options, the link argument, or a usage error.
func parseReply(args []string) (replyOpts, string, error) {
	var o replyOpts
	fs := newFlagSet("reply")
	o.register(fs)
	for _, name := range []string{"f", "file"} {
		fs.Var(&o.files, name, "")
	}
	fs.StringVar(&o.messageFile, "message-file", "", "")
	raw, err := onePositional(fs, args, "request link")
	if err != nil {
		return o, "", err
	}
	return o, raw, validateCommon(o.common)
}

// runReply answers someone's request: it checks the request is still open
// before reading anything, then encrypts the message and files to the
// requester's public key (protocol v3) and sends the one reply.
//
// Parameters:
//   - ctx: cancellation signal.
//   - args: arguments after "reply".
//
// Returns an error to classify.
func (a *app) runReply(ctx context.Context, args []string) error {
	o, raw, err := parseReply(args)
	if err != nil {
		return err
	}
	a.json = o.json
	if raw, err = a.readLinkArg(raw); err != nil {
		return err
	}
	link, err := envelope.ParseReplyLink(raw)
	if err != nil {
		return usagef("not a valid gone request link (quote it in single quotes so the shell keeps the #fragment)")
	}
	cl, err := newClient(a.env, link.Origin, "the link", o.common)
	if err != nil {
		return err
	}
	if _, err = cl.RequestOpen(ctx, link.ID, link.Fragment.Fill); err != nil {
		return replyGone(err)
	}
	return a.sendReply(ctx, cl, link, o)
}

// sendReply reads, seals and uploads the reply.
//
// Parameters:
//   - ctx: cancellation signal.
//   - cl: client for the link's server.
//   - link: the parsed request link.
//   - o: options.
//
// Returns an error to classify.
func (a *app) sendReply(ctx context.Context, cl *client.Client, link envelope.ReplyLink, o replyOpts) error {
	payload, err := a.readPayload(o.files, o.messageFile)
	defer clearPayload(payload)
	if err != nil {
		return err
	}
	packed, err := envelope.Pack(payload)
	defer clear(packed)
	if err != nil {
		return err
	}
	nonce, blob, err := envelope.SealV3(link.Fragment.PublicKey, packed)
	if err != nil {
		return err
	}
	expires, err := cl.Fill(ctx, link.ID, link.Fragment.Fill, nonce, blob)
	if err != nil {
		return replyGone(err)
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			Sent      bool   `json:"sent"`
			ExpiresAt string `json:"expires_at"`
		}{true, formatTime(expires)})
	}
	return writeText(a.env.Stdout, fmt.Sprintf("Sent. Only the requester can open it.\nExpires: %s\n", formatTime(expires)))
}

// replyGone turns a 404 on the reply route into the request-gone error.
//
// Parameters:
//   - err: client error.
//
// Returns the error to report.
func replyGone(err error) error {
	if errors.Is(err, client.ErrNotFound) {
		return &notFoundError{msg: msgReplyGone}
	}
	return err
}
