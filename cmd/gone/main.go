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

func loadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(2)
	}
	return cfg
}

func ensureDataDir(dir string) (string, string) {
	if st, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if mkErr := os.MkdirAll(dir, 0o600); mkErr != nil {
				slog.Error("failed to create data directory", "dir", dir, "err", mkErr)
				os.Exit(3)
			}
		} else {
			slog.Error("stat data directory", "dir", dir, "err", err)
			os.Exit(3)
		}
	} else if !st.IsDir() {
		slog.Error("data path not directory", "dir", dir)
		os.Exit(3)
	}
	blobDir := filepath.Join(dir, "blobs")
	if err := os.MkdirAll(blobDir, 0o600); err != nil {
		slog.Error("create blobs dir", "err", err)
		os.Exit(5)
	}
	return dir, blobDir
}

func openDatabase(dataDir string) (*sql.DB, store.Index) {
	dbPath := filepath.Join(dataDir, "gone.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		slog.Error("open sqlite driver", "err", err)
		os.Exit(4)
	}
	idx, err := sqlite.New(db)
	if err != nil {
		slog.Error("init sqlite schema", "err", err)
		os.Exit(4)
	}
	return db, idx
}

func newBlobStorage(blobDir string) store.BlobStorage {
	blobs, err := filesystem.New(blobDir)
	if err != nil {
		slog.Error("init blob storage", "err", err)
		os.Exit(5)
	}
	return blobs
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
func loadTemplates() (*templates, error) {
	partialsBytes, err := fs.ReadFile(wembed.Assets, "partials.tmpl.html")
	if err != nil {
		return nil, err
	}
	idx, about, secret, errorPage, err := parseAllPages(string(partialsBytes))
	if err != nil {
		return nil, err
	}
	return &templates{index: idx, about: about, secret: secret, errorPage: errorPage}, nil
}

func buildService(idx store.Index, blobs store.BlobStorage, cfg *config.Config, clock app.Clock) *app.Service {
	st := store.New(idx, blobs, clock, 1024*4)
	return &app.Service{Store: st, Clock: clock, MaxBytes: cfg.MaxBytes, MinTTL: cfg.MinTTL, MaxTTL: cfg.MaxTTL}
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
	return &http.Server{Addr: cfg.Addr, Handler: handler, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
}

func run() error {
	cfg := loadConfig()
	dataDir, blobDir := ensureDataDir(cfg.DataDir)
	db, idx := openDatabase(dataDir)
	defer db.Close()
	// Initialize metrics manager & schema early so other components can emit metrics.
	ctx := context.Background()
	mgr := metrics.New(db, metrics.Config{FlushInterval: 5 * time.Second, Logger: slog.Default()})
	if err := mgr.InitSchema(ctx); err != nil {
		return err
	}
	mgr.Start(ctx)
	defer mgr.Stop(context.Background())

	// Optional metrics server (separate listener) if configured.
	var metricsSrv *http.Server
	if cfg.MetricsAddr != "" {
		metricsSrv = &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler(mgr, cfg.MetricsToken), ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
		go func() {
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("metrics server error", "err", err)
			}
		}()
		slog.Info("metrics server started", "addr", cfg.MetricsAddr)
	}
	blobs := newBlobStorage(blobDir)
	clock := realClock{}
	svc := buildService(idx, blobs, cfg, clock)
	// Inject metrics into service (optional interface already defined)
	svc.Metrics = mgr
	tmpls, err := loadTemplates()
	if err != nil {
		return err
	}
	// Start janitor with metrics.
	janCfg := janitor.Config{Interval: time.Minute, Logger: slog.Default()}
	jan := janitor.New(store.New(idx, blobs, clock, 1024*4), mgr, janCfg) // reuse underlying components
	jan.Start(ctx)
	defer jan.Stop()

	srv := newServer(cfg, buildHandler(cfg, svc, db, blobDir, tmpls))
	slog.Info("starting server", "addr", cfg.Addr, "pid", os.Getpid())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(context.Background())
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
