// Package cli implements the gone command-line client. Run is the single
// entry point; all process state (streams, terminal, environment, config
// location) is injected through Env so every command is testable.
package cli

import (
	"crypto/x509"
	"io"
	"os"

	"golang.org/x/term"
)

// Env carries the process environment a command runs in.
type Env struct {
	// Stdin is the message source for send and the passphrase source for
	// prompts when StdinTTY is true.
	Stdin io.Reader
	// Stdout receives command results.
	Stdout io.Writer
	// Stderr receives prompts, notices and errors.
	Stderr io.Writer
	// StdinTTY reports whether Stdin is an interactive terminal.
	StdinTTY bool
	// StdoutTTY reports whether Stdout is an interactive terminal.
	StdoutTTY bool
	// ReadPassword reads one line from the terminal without echo.
	ReadPassword func() ([]byte, error)
	// Getenv looks up an environment variable.
	Getenv func(string) string
	// ConfigDir returns the per-user configuration root.
	ConfigDir func() (string, error)
	// Version is the build version reported by "gone version".
	Version string
	// RootCAs overrides the system trust store; nil uses the system pool.
	RootCAs *x509.CertPool
}

// OSEnv returns an Env bound to the real process.
//
// Parameters:
//   - version: build version string.
//
// Returns the environment.
func OSEnv(version string) *Env {
	in := int(os.Stdin.Fd())   // #nosec G115 -- file descriptors fit in int
	out := int(os.Stdout.Fd()) // #nosec G115 -- file descriptors fit in int
	return &Env{
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		StdinTTY:     term.IsTerminal(in),
		StdoutTTY:    term.IsTerminal(out),
		ReadPassword: func() ([]byte, error) { return term.ReadPassword(in) },
		Getenv:       os.Getenv,
		ConfigDir:    os.UserConfigDir,
		Version:      version,
	}
}
