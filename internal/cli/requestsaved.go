package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/haukened/gone/v3/internal/cli/reqdb"
	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/domain"
	"strings"
	"time"
)

// registerOutputs adds the flags that say where an opened reply goes.
//
// Parameters:
//   - o: options to fill.
//   - fs: flag set.
func registerOutputs(o *getOpts, fs flagAdder) {
	for _, name := range []string{"o", "out"} {
		fs.StringVar(&o.out, name, ".", "")
	}
	fs.BoolVar(&o.raw, "raw", false, "")
	fs.StringVar(&o.messageOut, "message-out", "", "")
}

// openRequestDB opens this device's request database.
//
// Returns the database or an I/O error.
func (a *app) openRequestDB() (*reqdb.DB, error) {
	dir, _, err := configPath(a.env)
	if err != nil {
		return nil, err
	}
	db, err := reqdb.Open(dir)
	if err != nil {
		return nil, ioErr("open the request database", err)
	}
	return db, nil
}

// hint writes a human-facing note to stderr, except in --json mode where
// stderr carries only the JSON error.
//
// Parameters:
//   - format: fmt format.
//   - args: format arguments.
func (a *app) hint(format string, args ...any) {
	if !a.json {
		_, _ = fmt.Fprintf(a.env.Stderr, format, args...)
	}
}

// resolveSaved finds a saved request by ID or unique prefix.
//
// Parameters:
//   - ctx: context.
//   - db: request database.
//   - arg: the ID argument.
//
// Returns the row, or a usage, not-found or I/O error.
func resolveSaved(ctx context.Context, db *reqdb.DB, arg string) (reqdb.Row, error) {
	row, err := db.Resolve(ctx, arg, time.Now())
	var amb *reqdb.AmbiguousError
	switch {
	case err == nil:
		return row, nil
	case errors.Is(err, reqdb.ErrBadPrefix):
		return row, usagef("%s", err.Error())
	case errors.Is(err, reqdb.ErrNoMatch):
		return row, &notFoundError{msg: "no saved request matches " + escapeTerminal(arg) + " (see \"gone request list\")"}
	case errors.As(err, &amb):
		return row, usagef("%s; use more characters: %s", amb.Error(), shortIDs(amb.IDs))
	}
	return row, ioErr("read the request database", err)
}

// shortIDs joins IDs shortened for display.
//
// Parameters:
//   - ids: full IDs.
//
// Returns the joined short IDs.
func shortIDs(ids []string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id[:shortID]
	}
	return strings.Join(out, ", ")
}

// savedTarget parses `request open|cancel ID` flags and resolves the row.
//
// Parameters:
//   - ctx: context.
//   - fs: flag set with the subcommand's flags registered.
//   - args: arguments after the subcommand.
//   - c: the parsed common flags.
//
// Returns the database (caller closes), the row, and its client.
func (a *app) savedTarget(ctx context.Context, fs *flag.FlagSet, args []string, c *common) (*reqdb.DB, reqdb.Row, *client.Client, error) {
	arg, err := onePositional(fs, args, "request ID")
	if err != nil {
		return nil, reqdb.Row{}, nil, err
	}
	if err = validateCommon(*c); err != nil {
		return nil, reqdb.Row{}, nil, err
	}
	a.json = c.json
	db, err := a.openRequestDB()
	if err != nil {
		return nil, reqdb.Row{}, nil, err
	}
	row, err := resolveSaved(ctx, db, arg)
	if err == nil {
		var cl *client.Client
		if cl, err = newClient(a.env, row.Origin, "the saved request's server", *c); err == nil {
			return db, row, cl, nil
		}
	}
	_ = db.Close()
	return nil, reqdb.Row{}, nil, err
}

// runRequestCancel implements `gone request cancel ID`.
//
// Parameters:
//   - ctx: cancellation signal.
//   - args: arguments after "cancel".
//
// Returns an error to classify.
func (a *app) runRequestCancel(ctx context.Context, args []string) error {
	var c common
	fs := newFlagSet("request cancel")
	c.register(fs)
	db, row, cl, err := a.savedTarget(ctx, fs, args, &c)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err = cl.CancelRequest(ctx, domain.SecretID(row.ID), domain.ManageToken(row.ManageToken)); err != nil {
		return a.forgetIfGone(ctx, db, row, err)
	}
	if err = db.Delete(ctx, row.ID); err != nil {
		return ioErr("update the request database", err)
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			Cancelled bool `json:"cancelled"`
		}{true})
	}
	return writeText(a.env.Stdout, "Cancelled.\n")
}

// forgetIfGone deletes a saved request the server no longer has, and
// turns its 404 into the request-gone error.
//
// Parameters:
//   - ctx: context.
//   - db: request database.
//   - row: the request.
//   - err: the client error.
//
// Returns the error to report.
func (a *app) forgetIfGone(ctx context.Context, db *reqdb.DB, row reqdb.Row, err error) error {
	if !errors.Is(err, client.ErrNotFound) {
		return err
	}
	if derr := db.Delete(ctx, row.ID); derr != nil {
		return ioErr("update the request database", derr)
	}
	return &notFoundError{msg: msgRequestGone}
}
