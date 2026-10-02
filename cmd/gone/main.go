// Package main provides the gone binary entry point that starts the HTTP server
// for one-time secret sharing. It loads configuration from environment variables
// and command-line flags, validates them, and then starts the HTTP server.
//
// The application flow:
//  1. Parse flags.
//  2. Load defaults and apply environment variables and flags.
//  3. Validate configuration.
//  4. Register minimal health endpoint.
//  5. Configure and start the HTTP server.
//
// It blocks until the server exits with an error (other than http.ErrServerClosed).
// main is the program entry point; it orchestrates configuration loading,
// validation, HTTP mux setup, and starts the HTTP server using the resolved
// configuration. It exits the process with a non-zero status code on
// configuration validation failure or fatal server errors.
package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"database/sql"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/httpx"
	"github.com/haukened/gone/internal/janitor"
	"github.com/haukened/gone/internal/metrics"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/filesystem"
	"github.com/haukened/gone/internal/store/sqlite"
	wembed "github.com/haukened/gone/web"
)

// realClock implements app.Clock using time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// privateDirPerm is owner-only access (rwx------). Directories need the
// execute (search) bit for the owner to open files inside them, so 0o700 is
// the most restrictive mode that still lets the service operate.
const privateDirPerm os.FileMode = 0o700

// ensureDataDir creates (if needed) the data directory and its blobs
// subdirectory, and enforces owner-only permissions on both, including
// directories that already existed with looser modes.
//
// Parameters:
//   - dir: path to the data directory.
//
// Returns:
//   - string: the data directory path.
//   - string: the blobs directory path.
//   - error: non-nil if a path is not a directory, cannot be created, or its
//     permissions cannot be restricted.
func ensureDataDir(dir string) (string, string, error) {
	if err := ensurePrivateDir(dir); err != nil {
		return "", "", fmt.Errorf("data dir: %w", err)
	}
	blobDir := filepath.Join(dir, "blobs")
	if err := ensurePrivateDir(blobDir); err != nil {
		return "", "", fmt.Errorf("blobs dir: %w", err)
	}
	return dir, blobDir, nil
}

// ensurePrivateDir creates dir if missing and forces its mode to
// privateDirPerm. MkdirAll does not alter pre-existing directories, so the
// mode is always re-applied explicitly.
//
// Parameters:
//   - dir: directory path to create or tighten.
//
// Returns:
//   - error: non-nil if dir exists but is not a directory, or if creation,
//     stat, or chmod fails.
func ensurePrivateDir(dir string) error {
	st, err := os.Stat(dir)
	// Directories need the owner execute (search) bit, so 0o700 is the
	// strictest usable mode; the rule below assumes file semantics.
	switch {
	case errors.Is(err, os.ErrNotExist):
		if mkErr := os.MkdirAll(dir, privateDirPerm); mkErr != nil { // nosemgrep: incorrect-default-permission
			return fmt.Errorf("create: %w", mkErr)
		}
	case err != nil:
		return fmt.Errorf("stat: %w", err)
	case !st.IsDir():
		return fmt.Errorf("not a directory: %s", dir)
	}
	if err := os.Chmod(dir, privateDirPerm); err != nil { // nosemgrep: incorrect-default-permission
		return fmt.Errorf("restrict permissions: %w", err)
	}
	return nil
}

// openDatabase opens <dataDir>/gone.db with the hardened DSN (WAL, foreign
// keys, busy timeout, FULL synchronous) and initializes the secrets schema.
//
// Parameters:
//   - dataDir: directory holding the SQLite database file.
//
// Returns:
//   - *sql.DB: the opened database handle (caller must Close).
//   - store.Index: SQLite-backed index over db.
//   - error: non-nil if the driver cannot open or the schema cannot be created.
func openDatabase(dataDir string) (*sql.DB, store.Index, error) {
	db, err := sql.Open(sqlite.DriverName, config.SQLiteDSNFor(dataDir))
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite driver: %w", err)
	}
	idx, err := sqlite.New(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("init sqlite schema: %w", err)
	}
	return db, idx, nil
}

func newBlobStorage(blobDir string) (store.BlobStorage, error) {
	blobs, err := filesystem.New(blobDir)
	if err != nil {
		return nil, fmt.Errorf("init blob storage: %w", err)
	}
	return blobs, nil
}

type templates struct{ index, about, secret, errorPage *template.Template }

// parsePage parses the base partials plus a single page template.
// Parameters:
//
//	base: the already-read partials template content as a string
//	name: the name to assign to the page template
//	file: the filename of the page template inside the embedded FS
//
// Returns the composed *template.Template or an error.
func parsePage(base, name, file string) (*template.Template, error) {
	pageBytes, err := fs.ReadFile(wembed.Assets, file)
	if err != nil {
		return nil, err
	}
	t, err := template.New("partials").Parse(base)
	if err != nil {
		return nil, err
	}
	return t.New(name).Parse(string(pageBytes))
}

// parseAllPages parses all known page templates returning individual templates.
// Splitting this out allows loadTemplates to remain very small and simple.
func parseAllPages(base string) (idx, about, secret, errorPage *template.Template, err error) {
	pages := []struct {
		name string
		file string
		out  **template.Template
	}{
		{"index", "index.tmpl.html", &idx},
		{"about", "about.tmpl.html", &about},
		{"secret", "secret.tmpl.html", &secret},
		{"error", "error.tmpl.html", &errorPage},
	}
	for _, p := range pages {
		var t *template.Template
		t, err = parsePage(base, p.name, p.file)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		*p.out = t
	}
	return idx, about, secret, errorPage, nil
}

// loadTemplates reads partials and composes individual page templates.
// Split into a helper to keep cyclomatic complexity low.
// loadTemplatesFrom loads templates from a provided filesystem. Exposed for tests.
func loadTemplatesFrom(fsys fs.FS) (*templates, error) {
	partialsBytes, err := fs.ReadFile(fsys, "partials.tmpl.html")
	if err != nil {
		return nil, err
	}
	idx, about, secret, errorPage, err := parseAllPages(string(partialsBytes))
	if err != nil {
		return nil, err
	}
	return &templates{index: idx, about: about, secret: secret, errorPage: errorPage}, nil
}

func loadTemplates() (*templates, error) { // retained for existing callers
	return loadTemplatesFrom(wembed.Assets)
}

func buildService(idx store.Index, blobs store.BlobStorage, cfg *config.Config, clock app.Clock) *app.Service {
	st := store.New(idx, blobs, clock, cfg.InlineMaxBytes)
	return &app.Service{Store: st, Clock: clock, MaxBytes: cfg.MaxBytes, MinTTL: cfg.MinTTL, MaxTTL: cfg.MaxTTL, ClaimLease: cfg.ClaimLease}
}

func buildHandler(cfg *config.Config, svc *app.Service, db *sql.DB, blobDir string, tmpls *templates) http.Handler {
	readiness := func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		if _, err := os.ReadDir(blobDir); err != nil {
			return err
		}
		return nil
	}
	h := httpx.New(svc, cfg.MaxBytes, readiness)
	h.IndexTmpl = httpx.TemplateRenderer{T: tmpls.index}
	h.AboutTmpl = httpx.AboutTemplateRenderer{T: tmpls.about}
	h.SecretTmpl = httpx.TemplateRenderer{T: tmpls.secret}
	if tmpls.errorPage != nil {
		h.ErrorTmpl = httpx.TemplateRenderer{T: tmpls.errorPage}
	}
	h.Assets = http.FS(wembed.Assets)
	h.MinTTL = cfg.MinTTL
	h.MaxTTL = cfg.MaxTTL
	h.TTLOptions = cfg.TTLOptions
	return h.Router()
}

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

// startMetrics initializes the metrics schema, starts the flush loop, and,
// when enabled, starts the token-protected metrics listener.
//
// Parameters:
//   - ctx: context for schema initialization and the flush loop.
//   - db: database the metrics manager persists to.
//   - cfg: configuration supplying the metrics address and token.
//
// Returns:
//   - *metrics.Manager: the started manager; the caller must Stop it.
//   - *http.Server: the metrics listener, or nil when metrics are disabled.
//   - error: non-nil if schema initialization fails.
func startMetrics(ctx context.Context, db *sql.DB, cfg *config.Config) (*metrics.Manager, *http.Server, error) {
	mgr := metrics.New(db, metrics.Config{FlushInterval: 5 * time.Second, Logger: slog.Default()})
	if err := mgr.InitSchema(ctx); err != nil {
		return nil, nil, err
	}
	mgr.Start(ctx)
	if cfg.MetricsAddr != "" && cfg.MetricsToken == "" {
		slog.Warn("metrics disabled: GONE_METRICS_ADDR set but GONE_METRICS_TOKEN is empty")
	}
	if !cfg.MetricsEnabled() {
		return mgr, nil, nil
	}
	srv := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler(mgr, cfg.MetricsToken), ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "err", err)
		}
	}()
	slog.Info("metrics server started", "addr", cfg.MetricsAddr)
	return mgr, srv, nil
}

// components bundles the long-lived components assembled by run.
type components struct {
	cfg     *config.Config
	db      *sql.DB
	idx     store.Index
	blobDir string
	mgr     *metrics.Manager
}

// serve wires the service, janitor, and HTTP server, then blocks serving
// requests until the listener stops.
//
// Parameters:
//   - ctx: context for the janitor loop.
//
// Returns:
//   - error: non-nil if blob storage, templates, or the listener fail.
func (a *components) serve(ctx context.Context) error {
	blobs, err := newBlobStorage(a.blobDir)
	if err != nil {
		return err
	}
	tmpls, err := loadTemplates()
	if err != nil {
		return err
	}
	clock := realClock{}
	svc := buildService(a.idx, blobs, a.cfg, clock)
	svc.Metrics = a.mgr
	janStore := store.New(a.idx, blobs, clock, a.cfg.InlineMaxBytes).WithMetrics(a.mgr)
	jan := janitor.New(janStore, a.mgr, janitor.Config{Interval: time.Minute, Logger: slog.Default()})
	jan.Start(ctx)
	defer jan.Stop()

	srv := newServer(a.cfg, buildHandler(a.cfg, svc, a.db, a.blobDir, tmpls))
	slog.Info("starting server", "addr", a.cfg.Addr, "pid", os.Getpid())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// run loads configuration, prepares storage and metrics, and serves until
// the HTTP listener exits.
//
// Returns:
//   - error: non-nil if any startup step or the listener fails.
func run() error {
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
	defer db.Close()
	ctx := context.Background()
	mgr, metricsSrv, err := startMetrics(ctx, db, cfg)
	if err != nil {
		return err
	}
	defer mgr.Stop(context.Background())
	if metricsSrv != nil {
		defer func() { _ = metricsSrv.Shutdown(context.Background()) }()
	}
	a := &components{cfg: cfg, db: db, idx: idx, blobDir: blobDir, mgr: mgr}
	return a.serve(ctx)
}

func main() {
	if err := run(); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
