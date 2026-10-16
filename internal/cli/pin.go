package cli

import (
	"flag"
	"slices"
	"strings"

	"github.com/haukened/gone/v3/internal/client"
)

// linkOpts holds the flags shared by commands that act on a link (get,
// reply, status, revoke).
type linkOpts struct {
	common
	pin pinOpts
}

// pinOpts decides which servers a link may send the CLI to.
type pinOpts struct {
	// server is the -s/--server value, or "".
	server string
	// allow holds the raw --allow-origin values.
	allow stringList
	// allowed holds the normalized --allow-origin origins after validate.
	allowed []string
}

// register adds the shared and origin flags to fs.
//
// Parameters:
//   - fs: flag set to extend.
func (l *linkOpts) register(fs *flag.FlagSet) {
	l.common.register(fs)
	for _, name := range []string{"s", "server"} {
		fs.StringVar(&l.pin.server, name, "", "")
	}
	fs.Var(&l.pin.allow, "allow-origin", "")
}

// validate checks the shared flags and normalizes every --allow-origin
// value, which may list several origins separated by commas. A bad value
// is never echoed: it might be a whole link pasted by mistake.
//
// Returns a usage error or nil.
func (l *linkOpts) validate() error {
	if err := validateCommon(l.common); err != nil {
		return err
	}
	for _, v := range strings.Split(l.pin.allow.String(), ",") {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		o, err := client.NormalizeOrigin(v, l.insecure)
		if err != nil {
			return originErr(err, "--allow-origin")
		}
		l.pin.allowed = append(l.pin.allowed, o)
	}
	return nil
}

// checkLinkOrigin refuses a link whose server is neither the configured
// server (resolved as for send) nor named by --allow-origin. It runs before
// any request, so a link crafted to point somewhere else learns nothing
// about the person opening it. Only origins are named in the error, never
// the link's ID or key.
//
// Parameters:
//   - origin: normalized origin of the link's client.
//   - p: validated origin flags.
//
// Returns nil, an untrusted-origin usage error, or a config error.
func (a *app) checkLinkOrigin(origin string, p pinOpts) error {
	raw, source, err := resolveServer(a.env, p.server)
	if err != nil {
		return err
	}
	server, err := client.CanonicalOrigin(raw)
	if err != nil {
		return originErr(err, source)
	}
	if origin == server || slices.Contains(p.allowed, origin) {
		return nil
	}
	return untrustedOrigin(escapeTerminal(origin), escapeTerminal(server), source)
}

// untrustedOrigin explains a refused link origin and how to trust it.
//
// Parameters:
//   - origin: the link's server.
//   - server: the server gone is set to use.
//   - source: where server came from ("default", "config", "--server",
//     or the environment variable name).
//
// Returns the usage error.
func untrustedOrigin(origin, server, source string) error {
	msg := "refused: this link is for " + origin + ", but gone is set to use " + server + " (from " + source + "); nothing was sent. "
	if source == "default" {
		msg = "refused: this link is for " + origin + ", but no server is set, so gone only trusts the default " + server + "; nothing was sent. "
	}
	msg += "If you trust " + origin + ", add --allow-origin " + origin + " for this command, or make it your server with: gone set server " + origin
	return &usageError{msg: msg, kind: "untrusted_origin"}
}
