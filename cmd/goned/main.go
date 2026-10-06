// Package main provides the goned server binary entry point that starts the HTTP server
// for one-time secret sharing.
//
// The application flow:
//  1. Load defaults and apply environment variables.
//  2. Validate configuration.
//  3. Prepare the private data directory, database, and blob storage.
//  4. Start metrics, the janitor, and the HTTP server.
//  5. On SIGINT/SIGTERM, shut down gracefully and flush metrics.
//
// "goned healthcheck" probes a running server's /readyz instead, for a
// container HEALTHCHECK.
//
// The process exits with a non-zero status code on configuration validation
// failure or fatal server errors.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/haukened/gone/v3/internal/buildinfo"
	"github.com/haukened/gone/v3/internal/config"
	wembed "github.com/haukened/gone/v3/web"
)

// version is the release this binary was built from, set at build time with
// -ldflags "-X main.version=vX.Y.Z". main falls back to the module version
// (go install); local builds report "dev" or a VCS pseudo-version.
var version = "dev"

// realClock implements app.Clock using time.Now.
type realClock struct{}

// Now returns the current time in UTC.
//
// Returns:
//   - time.Time: the current UTC time.
func (realClock) Now() time.Time { return time.Now().UTC() }

// loadConfig loads and validates configuration from defaults and the
// environment.
//
// Returns:
//   - *config.Config: the validated configuration.
//   - error: non-nil if loading or validation fails.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// run loads configuration, prepares storage and metrics, and serves until
// the HTTP listener fails or ctx is canceled.
//
// Parameters:
//   - ctx: canceling it shuts the servers down gracefully.
//
// Returns:
//   - error: non-nil if any startup step or the listener fails.
func run(ctx context.Context) (err error) {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	dataDir, blobDir, err := ensureDataDir(cfg.DataDir)
	if err != nil {
		return err
	}
	db, idx, err := openDatabase(dataDir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	// Canceling on return also stops the metrics listener and janitor when
	// the main listener fails rather than being signaled.
	ctx, cancel := context.WithCancel(ctx)
	mgr, err := startMetrics(ctx, db, cfg)
	if err != nil {
		cancel()
		return err
	}
	defer mgr.Stop(context.Background())
	defer cancel()
	c := &components{cfg: cfg, db: db, idx: idx, blobDir: blobDir, mgr: mgr, assets: wembed.Assets}
	return c.serve(ctx)
}

// main runs the service until SIGINT or SIGTERM and exits non-zero on error.
// "goned healthcheck" instead probes a running server and exits 0 if it is
// ready.
func main() {
	version = buildinfo.Version(version)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Stderr))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)
	stop()
	if err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
