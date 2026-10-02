package main

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/httpx"
	"github.com/haukened/gone/internal/janitor"
	"github.com/haukened/gone/internal/metrics"
	"github.com/haukened/gone/internal/store"
)

// shutdownTimeout bounds how long in-flight requests may take to finish once
// the process has been asked to stop.
const shutdownTimeout = 10 * time.Second

// buildService assembles the application service over the given storage.
//
// Parameters:
//   - idx: metadata index for secrets.
//   - blobs: filesystem storage for external payloads.
//   - cfg: configuration supplying size, TTL, and claim limits.
//   - clock: time source.
//
// Returns:
//   - *app.Service: the configured service (Metrics left unset).
func buildService(idx store.Index, blobs store.BlobStorage, cfg *config.Config, clock app.Clock) *app.Service {
	st := store.New(idx, blobs, clock, cfg.InlineMaxBytes)
	return &app.Service{Store: st, Clock: clock, MaxBytes: cfg.MaxBytes, MinTTL: cfg.MinTTL, MaxTTL: cfg.MaxTTL, ClaimLease: cfg.ClaimLease}
}

// readinessCheck reports whether the database answers pings and the blob
// directory is readable.
//
// Parameters:
//   - db: database to ping.
//   - blobDir: blob directory that must be listable.
//
// Returns:
//   - func(context.Context) error: probe returning nil when ready.
func readinessCheck(db *sql.DB, blobDir string) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		_, err := os.ReadDir(blobDir)
		return err
	}
}

// buildHandler wires the HTTP handler with templates, static assets, TTL
// options, and a readiness probe.
//
// Parameters:
//   - cfg: configuration supplying size and TTL limits.
//   - svc: application service backing the API.
//   - db: database used by the readiness probe.
//   - blobDir: blob directory used by the readiness probe.
//   - tmpls: parsed page templates; errorPage may be nil.
//   - assets: filesystem served under /static.
//
// Returns:
//   - *httpx.Handler: the configured handler; call Router to mount routes.
func buildHandler(cfg *config.Config, svc *app.Service, db *sql.DB, blobDir string, tmpls *templates, assets fs.FS) *httpx.Handler {
	h := httpx.New(svc, cfg.MaxBytes, readinessCheck(db, blobDir))
	h.IndexTmpl = httpx.TemplateRenderer{T: tmpls.index}
	h.AboutTmpl = httpx.AboutTemplateRenderer{T: tmpls.about}
	h.SecretTmpl = httpx.TemplateRenderer{T: tmpls.secret}
	if tmpls.errorPage != nil {
		h.ErrorTmpl = httpx.TemplateRenderer{T: tmpls.errorPage}
	}
	h.Assets = http.FS(assets)
	h.MinTTL = cfg.MinTTL
	h.MaxTTL = cfg.MaxTTL
	h.TTLOptions = cfg.TTLOptions
	return h
}

// newServer builds the public HTTP server with conservative timeouts.
//
// Parameters:
//   - cfg: configuration supplying the listen address.
//   - handler: root handler to serve.
//
// Returns:
//   - *http.Server: the unstarted server.
func newServer(cfg *config.Config, handler http.Handler) *http.Server {
	// Headers must arrive quickly (slowloris defense); body read/write windows
	// are sized so a full MaxBytes payload succeeds on modest connections.
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// listenAndServe runs srv until it fails or ctx is canceled, in which case
// it shuts down gracefully, waiting up to shutdownTimeout for in-flight
// requests.
//
// Parameters:
//   - ctx: cancellation signal that triggers graceful shutdown.
//   - srv: the server to run.
//
// Returns:
//   - error: the listener error, or the shutdown error; nil on clean stop.
func listenAndServe(ctx context.Context, srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		return shutdownServer(srv)
	}
}

// shutdownServer gracefully stops srv, waiting up to shutdownTimeout.
//
// Parameters:
//   - srv: the server to stop.
//
// Returns:
//   - error: non-nil if in-flight requests did not finish in time.
func shutdownServer(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(ctx)
}

// startMetrics initializes the metrics schema, starts the flush loop, and,
// when enabled, starts the token-protected metrics listener.
//
// Parameters:
//   - ctx: context for schema initialization, the flush loop, and the listener.
//   - db: database the metrics manager persists to.
//   - cfg: configuration supplying the metrics address and token.
//
// Returns:
//   - *metrics.Manager: the started manager; the caller must Stop it.
//   - error: non-nil if schema initialization fails.
func startMetrics(ctx context.Context, db *sql.DB, cfg *config.Config) (*metrics.Manager, error) {
	mgr := metrics.New(db, metrics.Config{FlushInterval: 5 * time.Second, Logger: slog.Default()})
	if err := mgr.InitSchema(ctx); err != nil {
		return nil, err
	}
	mgr.Start(ctx)
	if cfg.MetricsAddr != "" && cfg.MetricsToken == "" {
		slog.Warn("metrics disabled: GONE_METRICS_ADDR set but GONE_METRICS_TOKEN is empty")
	}
	if !cfg.MetricsEnabled() {
		return mgr, nil
	}
	srv := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler(mgr, cfg.MetricsToken), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := listenAndServe(ctx, srv); err != nil {
			slog.Error("metrics server error", "err", err)
		}
	}()
	slog.Info("metrics server started", "addr", cfg.MetricsAddr)
	return mgr, nil
}

// components bundles the long-lived components assembled by run.
type components struct {
	cfg     *config.Config
	db      *sql.DB
	idx     store.Index
	blobDir string
	mgr     *metrics.Manager
	assets  fs.FS
}

// serve wires the service, janitor, rate limiters, and HTTP server, then blocks serving
// requests until the listener fails or ctx is canceled.
//
// Parameters:
//   - ctx: context for the janitor loop; canceling it stops the server.
//
// Returns:
//   - error: non-nil if blob storage, templates, or the listener fail.
func (c *components) serve(ctx context.Context) error {
	blobs, err := newBlobStorage(c.blobDir)
	if err != nil {
		return err
	}
	tmpls, err := loadTemplatesFrom(c.assets)
	if err != nil {
		return err
	}
	clock := realClock{}
	svc := buildService(c.idx, blobs, c.cfg, clock)
	svc.Metrics = c.mgr
	janStore := store.New(c.idx, blobs, clock, c.cfg.InlineMaxBytes).WithMetrics(c.mgr)
	jan := janitor.New(janStore, c.mgr, janitor.Config{Interval: time.Minute, Logger: slog.Default()})
	jan.Start(ctx)
	defer jan.Stop()

	h := buildHandler(c.cfg, svc, c.db, c.blobDir, tmpls, c.assets)
	applyRateLimits(ctx, h, c.cfg, c.mgr)
	srv := newServer(c.cfg, h.Router())
	slog.Info("starting server", "addr", c.cfg.Addr, "pid", os.Getpid())
	return listenAndServe(ctx, srv)
}
