package cli

import (
	"errors"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/haukened/gone/v3/internal/client"
)

// stringList is a repeatable string flag.
type stringList []string

// String returns the values joined by commas.
//
// Returns the joined values.
func (s *stringList) String() string { return strings.Join(*s, ",") }

// Set appends a value.
//
// Parameters:
//   - v: flag value.
//
// Returns nil.
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// common holds flags shared by every networked command.
type common struct {
	json     bool
	insecure bool
	timeout  time.Duration
}

// register adds the shared flags to fs.
//
// Parameters:
//   - fs: flag set to extend.
func (c *common) register(fs *flag.FlagSet) {
	fs.BoolVar(&c.json, "json", false, "")
	fs.BoolVar(&c.insecure, "insecure", false, "")
	fs.DurationVar(&c.timeout, "timeout", client.DefaultTimeout, "")
}

// newFlagSet returns a quiet flag set; usage text comes from usage.go.
//
// Parameters:
//   - name: command name.
//
// Returns the flag set.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// parseArgs parses flags that may appear before or after positional
// arguments; "--" ends flag parsing.
//
// Parameters:
//   - fs: flag set.
//   - args: command arguments.
//
// Returns the positional arguments, flag.ErrHelp, or a usage error.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, usagef("%s: %v", fs.Name(), err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		// flag.Parse consumes a "--" terminator; everything after it is
		// positional.
		if used := len(args) - len(rest); used > 0 && args[used-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

// onePositional parses args and requires exactly one positional argument.
//
// Parameters:
//   - fs: flag set.
//   - args: command arguments.
//   - what: name of the positional argument for error messages.
//
// Returns the argument, flag.ErrHelp, or a usage error.
func onePositional(fs *flag.FlagSet, args []string, what string) (string, error) {
	pos, err := parseArgs(fs, args)
	if err != nil {
		return "", err
	}
	if len(pos) != 1 {
		return "", usagef("%s: expected exactly one %s", fs.Name(), what)
	}
	return pos[0], nil
}

// validateCommon checks shared flag values.
//
// Parameters:
//   - c: parsed shared flags.
//
// Returns a usage error for a non-positive timeout.
func validateCommon(c common) error {
	if c.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	return nil
}
