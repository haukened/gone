package cli

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/haukened/gone/v3/internal/cli/reqdb"
	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

const (
	// defaultRequestTTL is how long a request waits for a reply by default.
	defaultRequestTTL = time.Hour
	// maxLabelRunes caps a request's label, as in the web UI.
	maxLabelRunes = 80
	// shortID is how many ID characters the CLI shows.
	shortID = 8
)

// requestOpts holds the flags of `gone request` (create) and `request open`.
type requestOpts struct {
	getOpts
	server string
	ttl    time.Duration
	label  string
	wait   bool
}

// runRequest dispatches `gone request [list|open|cancel]`. With no
// subcommand it makes a new request.
//
// Parameters:
//   - ctx: cancellation signal.
//   - args: arguments after "request".
//
// Returns an error to classify.
func (a *app) runRequest(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return a.runRequestList(ctx, args[1:])
		case "open":
			return a.runRequestOpen(ctx, args[1:])
		case "cancel":
			return a.runRequestCancel(ctx, args[1:])
		}
	}
	return a.runRequestCreate(ctx, args)
}

// parseRequestCreate parses `gone request` flags.
//
// Parameters:
//   - args: arguments after "request".
//
// Returns the options or a usage error.
func parseRequestCreate(args []string) (requestOpts, error) {
	var o requestOpts
	fs := newFlagSet("request")
	o.common.register(fs)
	registerOutputs(&o.getOpts, fs)
	registerServerTTL(fs, &o.server, &o.ttl, defaultRequestTTL)
	for _, name := range []string{"l", "label"} {
		fs.StringVar(&o.label, name, "", "")
	}
	fs.BoolVar(&o.wait, "wait", false, "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return o, err
	}
	if len(pos) > 0 {
		return o, usagef("request: unknown subcommand %q (want list, open or cancel)", pos[0])
	}
	if o.ttl <= 0 {
		return o, usagef("--ttl must be positive")
	}
	o.label = cleanLabel(o.label)
	return o, validateCommon(o.common)
}

// cleanLabel trims a label, repairs invalid UTF-8 and caps its length.
//
// Parameters:
//   - s: the label as typed.
//
// Returns the stored label.
func cleanLabel(s string) string {
	s = strings.TrimSpace(strings.ToValidUTF8(s, "�"))
	if utf8.RuneCountInString(s) <= maxLabelRunes {
		return s
	}
	return string([]rune(s)[:maxLabelRunes])
}

// runRequestCreate makes a request: a key pair here, the request on the
// server, then the row saved locally before anything is printed. With
// --wait it then waits for the reply and opens it.
//
// Parameters:
//   - ctx: cancellation signal.
//   - args: arguments after "request".
//
// Returns an error to classify.
func (a *app) runRequestCreate(ctx context.Context, args []string) error {
	o, err := parseRequestCreate(args)
	if err != nil {
		return err
	}
	a.json = o.json
	cl, err := a.serverClient(o.server, o.common)
	if err != nil {
		return err
	}
	out, err := a.waitOutputs(o)
	if err != nil {
		return err
	}
	defer out.close()
	db, err := a.openRequestDB()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	row, err := a.makeRequest(ctx, cl, db, o)
	if err != nil {
		return err
	}
	defer clear(row.PrivateKey)
	if !o.wait {
		a.hint("Send the link to the person who has the secret. Open the reply with: gone request open %s\n", row.ID[:shortID])
		return nil
	}
	return a.waitAndOpen(ctx, cl, db, row, out, o.getOpts, true)
}

// waitOutputs checks the reply's output destinations up front when the
// command will open it, so nothing is created on the server if they can't
// be used.
//
// Parameters:
//   - o: options.
//
// Returns the outputs (zero when not waiting) or an error.
func (a *app) waitOutputs(o requestOpts) (outputs, error) {
	if !o.wait {
		return outputs{}, nil
	}
	return openOutputs(o.getOpts)
}

// makeRequest creates the request on the server, saves it, and reports
// the reply link. If saving fails the request is cancelled, since nothing
// could ever open its reply. The caller clears the returned private key.
//
// Parameters:
//   - ctx: cancellation signal.
//   - cl: client for the chosen server.
//   - db: request database.
//   - o: options.
//
// Returns the saved row or an error.
func (a *app) makeRequest(ctx context.Context, cl *client.Client, db *reqdb.DB, o requestOpts) (reqdb.Row, error) {
	priv, err := envelope.NewRequestKey()
	if err != nil {
		return reqdb.Row{}, err
	}
	created, err := cl.CreateRequest(ctx, o.ttl)
	if err != nil {
		return reqdb.Row{}, err
	}
	frag, err := envelope.NewReplyFragment(priv.PublicKey().Bytes(), created.FillToken.String())
	if err != nil {
		return reqdb.Row{}, err
	}
	row := reqdb.Row{
		ID: created.ID.String(), Origin: cl.Origin(), Label: o.label, ManageToken: created.ManageToken.String(),
		PrivateKey: priv.Bytes(), ReplyLink: envelope.ReplyLink{Origin: cl.Origin(), ID: created.ID, Fragment: frag}.String(),
		State: reqdb.StateWaiting, CreatedAt: time.Now().UTC(), ExpiresAt: created.ExpiresAt.UTC(),
	}
	if err = db.Put(ctx, row); err != nil {
		clear(row.PrivateKey)
		_ = cl.CancelRequest(context.WithoutCancel(ctx), created.ID, created.ManageToken)
		return reqdb.Row{}, ioErr("save the request", err)
	}
	return row, a.reportRequest(row)
}

// reportRequest prints a new request's link.
//
// Parameters:
//   - row: the saved request.
//
// Returns a write error, if any.
func (a *app) reportRequest(row reqdb.Row) error {
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			ID        string `json:"id"`
			Link      string `json:"link"`
			Label     string `json:"label"`
			ExpiresAt string `json:"expires_at"`
		}{row.ID, row.ReplyLink, row.Label, formatTime(row.ExpiresAt)})
	}
	return writeText(a.env.Stdout, fmt.Sprintf("Link:    %s\nID:      %s\nExpires: %s\n", row.ReplyLink, row.ID, formatTime(row.ExpiresAt)))
}
