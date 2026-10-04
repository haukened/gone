package cli

import (
	"errors"
	"fmt"

	"github.com/haukened/gone/internal/client"
)

// runSet implements "gone set server <url> [--insecure]".
//
// Parameters:
//   - args: arguments after "set".
//
// Returns a usage, I/O, or nil error.
func (a *app) runSet(args []string) error {
	fs := newFlagSet("set")
	insecure := fs.Bool("insecure", false, "")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || pos[0] != "server" {
		return usagef("usage: gone set server <url> [--insecure]")
	}
	origin, err := client.NormalizeOrigin(pos[1], *insecure)
	if errors.Is(err, client.ErrInsecureOrigin) {
		return usagef("%q uses http; pass --insecure to allow unencrypted transport", pos[1])
	}
	if err != nil {
		return usagef("%q is not a valid server origin (want https://host[:port])", pos[1])
	}
	path, err := saveConfig(a.env, config{Server: origin})
	if err != nil {
		return err
	}
	return writeText(a.env.Stdout, fmt.Sprintf("Server set to %s (%s)\n", origin, path))
}
