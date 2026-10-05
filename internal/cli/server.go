package cli

import (
	"errors"

	"github.com/haukened/gone/v3/internal/client"
)

const (
	// defaultServer is used when no flag, environment variable or config
	// file names a server.
	defaultServer = "https://gone.hauken.us"
	// serverEnv names the environment variable that overrides the config.
	serverEnv = "GONE_SERVER"
)

// resolveServer picks the server for send: the flag, then GONE_SERVER,
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
	switch {
	case errors.Is(err, client.ErrInsecureOrigin):
		return nil, usagef("%s uses http; Gone only talks to https servers (a localhost server may use http with --insecure)", source)
	case err != nil:
		return nil, usagef("%s is not a valid server origin (want https://host[:port])", source)
	}
	return cl, nil
}
