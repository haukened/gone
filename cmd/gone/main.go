// Command gone is the command-line client for a Gone server: it encrypts
// secrets locally, uploads them, and opens one-time links.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/haukened/gone/v3/internal/buildinfo"
	"github.com/haukened/gone/v3/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...". Builds
// without it, such as go install, report the module version instead.
var version = "dev"

// main runs the CLI and exits with its status code.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], cli.OSEnv(buildinfo.Version(version)))
	stop()
	os.Exit(code)
}
