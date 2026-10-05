package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/domain"
)

// defaultTTL is the secret lifetime when --ttl is not given.
const defaultTTL = time.Hour

// sendOpts holds the parsed "gone send" flags.
type sendOpts struct {
	common
	server string
	ttl    time.Duration
	files  stringList
	pass   passOpts
}

// parseSend parses "gone send" arguments.
//
// Parameters:
//   - args: arguments after "send".
//
// Returns the options, flag.ErrHelp, or a usage error.
func parseSend(args []string) (sendOpts, error) {
	var o sendOpts
	fs := newFlagSet("send")
	o.register(fs)
	o.pass.register(fs, true)
	for _, name := range []string{"s", "server"} {
		fs.StringVar(&o.server, name, "", "")
	}
	for _, name := range []string{"t", "ttl"} {
		fs.DurationVar(&o.ttl, name, defaultTTL, "")
	}
	for _, name := range []string{"f", "file"} {
		fs.Var(&o.files, name, "")
	}
	pos, err := parseArgs(fs, args)
	if err != nil {
		return o, err
	}
	if len(pos) > 0 {
		return o, usagef("send takes no arguments; pipe the message on stdin (see \"gone help send\")")
	}
	if o.ttl <= 0 {
		return o, usagef("--ttl must be positive")
	}
	return o, validateCommon(o.common)
}

// runSend implements "gone send".
//
// Parameters:
//   - ctx: request context.
//   - args: arguments after "send".
//
// Returns nil or a classified error.
func (a *app) runSend(ctx context.Context, args []string) error {
	o, err := parseSend(args)
	if err != nil {
		return err
	}
	a.json = o.json
	cl, err := a.sendClient(o)
	if err != nil {
		return err
	}
	return a.createSendSecret(ctx, cl, o)
}

// sendClient builds the API client for send options.
//
// Parameters:
//   - o: parsed send options.
//
// Returns the client or an origin/configuration error.
func (a *app) sendClient(o sendOpts) (*client.Client, error) {
	origin, source, err := resolveServer(a.env, o.server)
	if err != nil {
		return nil, err
	}
	return newClient(a.env, origin, source, o.common)
}

// createSendSecret reads, encrypts, uploads, and reports one send request.
//
// Parameters:
//   - ctx: request context.
//   - cl: API client.
//   - o: parsed send options.
//
// Returns nil or a classified error.
func (a *app) createSendSecret(ctx context.Context, cl *client.Client, o sendOpts) error {
	pass, generated, err := a.sendPassphrase(o.pass)
	defer clear(pass)
	if err != nil {
		return err
	}
	payload, err := a.readPayload(o.files)
	defer clearPayload(payload)
	if err != nil {
		return err
	}
	s, err := sealPayload(payload, pass)
	if err != nil {
		return err
	}
	defer clear(s.key)
	res, err := cl.Create(ctx, client.CreateRequest{Version: s.version, Nonce: s.nonce, TTL: o.ttl, Body: s.body})
	if err != nil {
		return err
	}
	return a.reportSend(cl.Origin(), res, s, generated, pass)
}

// reportSend prints the links for a stored secret.
//
// Parameters:
//   - origin: server origin.
//   - res: create result.
//   - s: sealed payload.
//   - generated: whether pass was generated and must be shown.
//   - pass: passphrase, or nil.
//
// Returns an output error, if any.
func (a *app) reportSend(origin string, res client.CreateResult, s sealed, generated bool, pass []byte) error {
	link, manage, err := buildLinks(origin, res.ID, s, res.ManageToken)
	if err != nil {
		return err
	}
	if generated {
		_, _ = fmt.Fprintf(a.env.Stderr, "Passphrase (share it separately from the link): %s\n", pass)
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			Link       string          `json:"link"`
			ManageLink string          `json:"manage_link"`
			ID         domain.SecretID `json:"id"`
			ExpiresAt  time.Time       `json:"expires_at"`
		}{link, manage, res.ID, res.ExpiresAt.UTC()})
	}
	return writeText(a.env.Stdout, fmt.Sprintf("Link:        %s\nManage link: %s\nExpires:     %s\n",
		link, manage, formatTime(res.ExpiresAt)))
}
