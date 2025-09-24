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

type templates struct{ index, about, secret *template.Template }

// tplSpec describes a template file to parse with a name added to the base partials template.
type tplSpec struct{ name, file string }

// loadTemplates parses partials plus page templates using a generic loop to avoid duplication.
func loadTemplates() (*templates, error) {
	partialsBytes, err := fs.ReadFile(wembed.FS, "partials.tmpl.html")
	if err != nil {
		return nil, err
	}
	base := string(partialsBytes)
	specs := []tplSpec{{"index", "index.tmpl.html"}, {"about", "about.tmpl.html"}, {"secret", "secret.tmpl.html"}}
	out := &templates{}
	for _, spec := range specs {
		pageBytes, err := fs.ReadFile(wembed.FS, spec.file)
		if err != nil {
			return nil, err
		}
		t, err := template.New("partials").Parse(base)
		if err == nil {
			t, err = t.New(spec.name).Parse(string(pageBytes))
		}
		if err != nil {
			return nil, err
		}
		switch spec.name {
		case "index":
			out.index = t
		case "about":
			out.about = t
		case "secret":
			out.secret = t
		}
	}
	return out, nil
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
	h.Assets = http.FS(wembed.FS)
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
	blobs := newBlobStorage(blobDir)
	clock := realClock{}
	svc := buildService(idx, blobs, cfg, clock)
	tmpls, err := loadTemplates()
	if err != nil {
		return err
	}
	srv := newServer(cfg, buildHandler(cfg, svc, db, blobDir, tmpls))
	slog.Info("starting server", "addr", cfg.Addr, "pid", os.Getpid())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
