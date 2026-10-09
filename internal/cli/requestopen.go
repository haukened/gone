package cli

import (
	"context"
	"crypto/ecdh"
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/haukened/gone/v3/internal/cli/reqdb"
	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// pollInterval is how often --wait checks a request. Status is read-only
// on the server, so polling costs it no writes. Tests shorten it.
var pollInterval = 10 * time.Second

// errNoReplyYet reports a request that has not been answered yet.
var errNoReplyYet = errors.New("no reply yet; run with --wait to wait for it")

// runRequestOpen implements `gone request open ID`.
//
// Parameters:
//   - ctx: cancellation signal.
//   - args: arguments after "open".
//
// Returns an error to classify.
func (a *app) runRequestOpen(ctx context.Context, args []string) error {
	var o getOpts
	var wait bool
	fs := newFlagSet("request open")
	o.register(fs)
	registerOutputs(&o, fs)
	fs.BoolVar(&wait, "wait", false, "")
	db, row, cl, err := a.savedTarget(ctx, fs, args, &o.common)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	out, err := openOutputs(o)
	if err != nil {
		return err
	}
	defer out.close()
	return a.waitAndOpen(ctx, cl, db, row, out, o, wait)
}

// waitAndOpen waits for the reply (when asked to), then opens it.
//
// Parameters:
//   - ctx: cancellation signal.
//   - cl: client for the request's server.
//   - db: request database.
//   - row: the request.
//   - out: where the reply goes.
//   - o: output options.
//   - wait: whether to wait for a reply that hasn't arrived.
//
// Returns an error to classify.
func (a *app) waitAndOpen(ctx context.Context, cl *client.Client, db *reqdb.DB, row reqdb.Row, out outputs, o getOpts, wait bool) error {
	if err := a.awaitReply(ctx, cl, db, row, wait); err != nil {
		return err
	}
	return a.openReply(ctx, cl, db, row, out, o)
}

// awaitReply returns once the request's reply is ready. Without wait it
// checks once.
//
// Parameters:
//   - ctx: cancellation signal.
//   - cl: client.
//   - db: request database.
//   - row: the request.
//   - wait: whether to keep polling.
//
// Returns nil when ready, errNoReplyYet, the request-gone error, or another
// error.
func (a *app) awaitReply(ctx context.Context, cl *client.Client, db *reqdb.DB, row reqdb.Row, wait bool) error {
	if !wait {
		delay, err := a.checkReply(ctx, cl, db, row)
		if err == nil && delay > 0 {
			return errNoReplyYet
		}
		return err
	}
	a.hint("Waiting for the reply (Ctrl-C to stop; the request stays saved)...\n")
	for {
		delay, err := a.checkReply(ctx, cl, db, row)
		if delay == 0 {
			return err
		}
		if err = sleepCtx(ctx, delay); err != nil {
			a.hint("Stopped waiting. Resume with: gone request open %s --wait\n", row.ID[:shortID])
			return err
		}
	}
}

// checkReply asks the server about the request once.
//
// Parameters:
//   - ctx: context.
//   - cl: client.
//   - db: request database.
//   - row: the request.
//
// Returns (0, nil) when ready; a positive delay when worth checking again
// (with the error that caused it, if any); or (0, err) for a final error.
func (a *app) checkReply(ctx context.Context, cl *client.Client, db *reqdb.DB, row reqdb.Row) (time.Duration, error) {
	st, err := cl.RequestStatus(ctx, domain.SecretID(row.ID), domain.ManageToken(row.ManageToken))
	var rl *client.RateLimitError
	switch {
	case err == nil && st.Ready:
		_ = db.SetState(ctx, row.ID, reqdb.StateReady, st.ExpiresAt)
		return 0, nil
	case err == nil:
		return pollInterval, nil
	case errors.As(err, &rl):
		return max(rl.RetryAfter, pollInterval), err
	case errors.Is(err, client.ErrNetwork), errors.Is(err, client.ErrServer):
		return pollInterval, err
	}
	return 0, a.forgetIfGone(ctx, db, row, err)
}

// sleepCtx waits for d or until ctx is done.
//
// Parameters:
//   - ctx: cancellation signal.
//   - d: how long to wait.
//
// Returns ctx.Err() if cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// openReply claims the reply, decrypts it with the saved key, delivers it
// exactly as `gone get` would, acknowledges it, and forgets the request.
//
// Parameters:
//   - ctx: context.
//   - cl: client.
//   - db: request database.
//   - row: the request.
//   - out: where the reply goes.
//   - o: output options.
//
// Returns an error to classify.
func (a *app) openReply(ctx context.Context, cl *client.Client, db *reqdb.DB, row reqdb.Row, out outputs, o getOpts) error {
	id := domain.SecretID(row.ID)
	claimed, err := cl.ClaimReply(ctx, id, domain.ManageToken(row.ManageToken))
	if err != nil {
		return a.forgetIfGone(ctx, db, row, err)
	}
	defer clear(claimed.Body)
	plain, err := openWithSavedKey(row.PrivateKey, claimed)
	defer clear(plain)
	if err != nil {
		return err
	}
	if err = a.deliver(out, o, plain); err != nil {
		return err
	}
	ackErr := cl.AckReply(ctx, id, claimed.Token)
	if err = db.Delete(ctx, row.ID); err != nil {
		return ioErr("update the request database", err)
	}
	if ackErr != nil {
		return &ackError{err: ackErr}
	}
	return nil
}

// openWithSavedKey decrypts a claimed reply with the saved P-256 scalar.
//
// Parameters:
//   - scalar: the raw private key; cleared before returning.
//   - claimed: the claimed reply.
//
// Returns the plaintext or envelope.ErrDecrypt.
func openWithSavedKey(scalar []byte, claimed client.Claimed) ([]byte, error) {
	defer clear(scalar)
	priv, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		return nil, envelope.ErrDecrypt
	}
	return envelope.OpenV3(priv, claimed.Nonce, claimed.Body)
}

// listedRequest is one row of `gone request list --json`.
type listedRequest struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	State     string `json:"state"`
	Server    string `json:"server"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Link      string `json:"link"`
}

// runRequestList implements `gone request list`.
//
// Parameters:
//   - ctx: context.
//   - args: arguments after "list".
//
// Returns an error to classify.
func (a *app) runRequestList(ctx context.Context, args []string) error {
	var c common
	var offline bool
	fs := newFlagSet("request list")
	c.register(fs)
	fs.BoolVar(&offline, "offline", false, "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("request list takes no arguments")
	}
	if err = validateCommon(c); err != nil {
		return err
	}
	a.json = c.json
	db, err := a.openRequestDB()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.List(ctx, time.Now())
	if err != nil {
		return ioErr("read the request database", err)
	}
	return a.printRequests(a.refreshAll(ctx, db, rows, c, offline))
}

// refreshAll asks each request's server for its state, unless offline. A
// request the server no longer has is shown as gone once and forgotten; a
// check that fails keeps the last known state, marked unknown.
//
// Parameters:
//   - ctx: context.
//   - db: request database.
//   - rows: saved requests.
//   - c: common flags (for --insecure and --timeout).
//   - offline: skip the server.
//
// Returns the rows to print.
func (a *app) refreshAll(ctx context.Context, db *reqdb.DB, rows []reqdb.Row, c common, offline bool) []listedRequest {
	out := make([]listedRequest, 0, len(rows))
	failed := false
	for _, r := range rows {
		state := r.State
		if !offline {
			state = a.refreshOne(ctx, db, &r, c)
		}
		failed = failed || state == "unknown"
		out = append(out, listedRequest{
			ID: r.ID, Label: r.Label, State: state, Server: r.Origin,
			CreatedAt: formatTime(r.CreatedAt), ExpiresAt: formatTime(r.ExpiresAt), Link: r.ReplyLink,
		})
	}
	if failed {
		a.hint("Some requests couldn't be checked; their state is unknown. Try again later.\n")
	}
	return out
}

// refreshOne checks one request and records what changed.
//
// Parameters:
//   - ctx: context.
//   - db: request database.
//   - r: the request; its expiry is updated in place.
//   - c: common flags.
//
// Returns "waiting", "ready", "gone", or "unknown".
func (a *app) refreshOne(ctx context.Context, db *reqdb.DB, r *reqdb.Row, c common) string {
	cl, err := newClient(a.env, r.Origin, "a saved request's server", c)
	if err != nil {
		return "unknown"
	}
	st, err := cl.RequestStatus(ctx, domain.SecretID(r.ID), domain.ManageToken(r.ManageToken))
	if errors.Is(err, client.ErrNotFound) {
		_ = db.Delete(ctx, r.ID)
		return "gone"
	}
	if err != nil {
		return "unknown"
	}
	state := reqdb.StateWaiting
	if st.Ready {
		state = reqdb.StateReady
	}
	r.ExpiresAt = st.ExpiresAt.UTC()
	_ = db.SetState(ctx, r.ID, state, r.ExpiresAt)
	return state
}

// stateLabels are the text-table spellings of each state.
var stateLabels = map[string]string{
	reqdb.StateWaiting: "waiting",
	reqdb.StateReady:   "reply ready",
	"gone":             "gone",
	"unknown":          "unknown",
}

// printRequests writes the list as JSON or an aligned table.
//
// Parameters:
//   - rows: rows to print.
//
// Returns a write error, if any.
func (a *app) printRequests(rows []listedRequest) error {
	if a.json {
		return writeJSON(a.env.Stdout, rows)
	}
	if len(rows) == 0 {
		return writeText(a.env.Stdout, "No requests saved on this device. Make one with: gone request\n")
	}
	tw := tabwriter.NewWriter(a.env.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tLABEL\tSTATE\tASKED\tEXPIRES")
	for _, r := range rows {
		label := r.Label
		if label == "" {
			label = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID[:shortID], escapeTerminal(label), stateLabels[r.State],
			tableTime(r.CreatedAt), tableTime(r.ExpiresAt))
	}
	if err := tw.Flush(); err != nil {
		return ioErr("write output", err)
	}
	return nil
}

// tableTime shortens an RFC 3339 UTC time for the table.
//
// Parameters:
//   - s: RFC 3339 time.
//
// Returns "2006-01-02 15:04 UTC", or s unchanged if it doesn't parse.
func tableTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
