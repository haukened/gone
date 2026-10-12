package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/haukened/gone/v3/internal/cli/reqdb"
	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/domain"
	"text/tabwriter"
	"time"
)

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
