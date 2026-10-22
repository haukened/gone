package cli

import (
	"errors"
	"fmt"

	"github.com/haukened/gone/v3/internal/client"
)

const (
	// defaultServer is used when no flag, environment variable or config
	// file names a server.
	defaultServer = "https://app.getgone.dev"
	// serverEnv names the environment variable that overrides the config.
	serverEnv = "GONE_SERVER"
)

// resolveServer picks the server for send, and the one links are checked
// against: the flag, then GONE_SERVER,
// then the config file, then defaultServer.
//
// Parameters:
//   - env: process environment.
//   - flagValue: value of -s/--server, or "".
//
// Returns the raw origin and a label naming its source, or an I/O error
// reading the config.
func resolveServer(env *Env, flagValue string) (string, string, error) {
	if flagValue != "" {
		return flagValue, "--server", nil
	}
	if v := env.Getenv(serverEnv); v != "" {
		return v, serverEnv, nil
	}
	c, err := loadConfig(env)
	if err != nil {
		return "", "", err
	}
	if c.Server != "" {
		return c.Server, "config", nil
	}
	return defaultServer, "default", nil
}

// serverClient builds the client for send and request: the server from
// resolveServer. When --server or GONE_SERVER overrides a server saved
// with "gone set server", a one-line notice names the origin in use.
//
// Parameters:
//   - flagValue: value of -s/--server, or "".
//   - c: shared flags.
//
// Returns the client, or a usage or config error.
func (a *app) serverClient(flagValue string, c common) (*client.Client, error) {
	origin, source, err := resolveServer(a.env, flagValue)
	if err != nil {
		return nil, err
	}
	cl, err := newClient(a.env, origin, source, c)
	if err != nil {
		return nil, err
	}
	a.noticeOverride(cl.Origin(), source)
	return cl, nil
}

// noticeOverride tells the user on stderr when the server in use, picked
// by --server or GONE_SERVER, differs from the configured one. Origins are
// not secret. A config that cannot be read is ignored here: send with an
// explicit server never needed it.
//
// Parameters:
//   - origin: normalized origin in use.
//   - source: where it came from.
func (a *app) noticeOverride(origin, source string) {
	if source == "config" || source == "default" {
		return
	}
	c, err := loadConfig(a.env)
	if err != nil || c.Server == "" || c.Server == origin {
		return
	}
	_, _ = fmt.Fprintf(a.env.Stderr, "Using %s (from %s), not the configured server %s.\n",
		escapeTerminal(origin), source, escapeTerminal(c.Server))
}

// newClient builds an API client, turning origin errors into usage errors
// that name where the origin came from.
//
// Parameters:
//   - env: process environment.
//   - origin: server origin.
//   - source: label for error messages.
//   - c: shared flags.
//
// Returns the client or a usage error.
func newClient(env *Env, origin, source string, c common) (*client.Client, error) {
	cl, err := client.New(origin, client.Options{
		AllowHTTP: c.insecure,
		Timeout:   c.timeout,
		UserAgent: "gone-cli/" + env.Version,
		RootCAs:   env.RootCAs,
	})
	if err != nil {
		return nil, originErr(err, source)
	}
	return cl, nil
}

// originErr turns an origin validation error into a usage error naming
// where the origin came from, never the value itself.
//
// Parameters:
//   - err: non-nil error from client.New or client.NormalizeOrigin.
//   - source: label for the message.
//
// Returns the usage error.
func originErr(err error, source string) error {
	if errors.Is(err, client.ErrInsecureOrigin) {
		return usagef("%s uses http; Gone only talks to https servers (a server on localhost or a loopback IP may use http with --insecure)", source)
	}
	return usagef("%s is not a valid server origin (want https://host[:port])", source)
}
