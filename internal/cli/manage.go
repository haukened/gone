package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// manageTarget parses a manage-link command line and connects to its
// origin.
//
// Parameters:
//   - name: command name.
//   - args: command arguments.
//
// Returns the client, the parsed link, flag.ErrHelp, or a usage error.
func (a *app) manageTarget(name string, args []string) (*client.Client, envelope.ManageLink, error) {
	var c common
	fs := newFlagSet(name)
	c.register(fs)
	raw, err := onePositional(fs, args, "manage link")
	if err != nil {
		return nil, envelope.ManageLink{}, err
	}
	if err := validateCommon(c); err != nil {
		return nil, envelope.ManageLink{}, err
	}
	if raw, err = a.readLinkArg(raw); err != nil {
		return nil, envelope.ManageLink{}, err
	}
	link, err := envelope.ParseManageLink(raw)
	if err != nil {
		return nil, envelope.ManageLink{}, usagef("%s: not a valid manage link", name)
	}
	cl, err := newClient(a.env, link.Origin, "the manage link", c)
	if err != nil {
		return nil, envelope.ManageLink{}, err
	}
	a.json = c.json
	return cl, link, nil
}

// runStatus implements "gone status <manage-link>".
//
// Parameters:
//   - ctx: request context.
//   - args: arguments after "status".
//
// Returns nil or a classified error.
func (a *app) runStatus(ctx context.Context, args []string) error {
	cl, link, err := a.manageTarget("status", args)
	if err != nil {
		return err
	}
	st, err := cl.Status(ctx, link.ID, link.Token)
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			State     string    `json:"state"`
			CreatedAt time.Time `json:"created_at"`
			ExpiresAt time.Time `json:"expires_at"`
		}{st.State, st.CreatedAt.UTC(), st.ExpiresAt.UTC()})
	}
	return writeText(a.env.Stdout, fmt.Sprintf("State:   %s\nCreated: %s\nExpires: %s\n",
		escapeTerminal(st.State), formatTime(st.CreatedAt), formatTime(st.ExpiresAt)))
}

// runRevoke implements "gone revoke <manage-link>".
//
// Parameters:
//   - ctx: request context.
//   - args: arguments after "revoke".
//
// Returns nil or a classified error.
func (a *app) runRevoke(ctx context.Context, args []string) error {
	cl, link, err := a.manageTarget("revoke", args)
	if err != nil {
		return err
	}
	if err := cl.Revoke(ctx, link.ID, link.Token); err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.env.Stdout, struct {
			Revoked bool `json:"revoked"`
		}{true})
	}
	return writeText(a.env.Stdout, "Revoked.\n")
}

// formatTime renders t for humans in UTC.
//
// Parameters:
//   - t: timestamp.
//
// Returns an RFC 3339 string.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
