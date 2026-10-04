// Command gone is the command-line client for a Gone server: it encrypts
// secrets locally, uploads them, and opens one-time links.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/haukened/gone/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// main runs the CLI and exits with its status code.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], cli.OSEnv(version))
	stop()
	os.Exit(code)
}
