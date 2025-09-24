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

func main() {
	// Load and validate configuration.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(2)
	}

	// ensure the data directory exists with secure permissions (owner rwx)
	if st, err := os.Stat(cfg.DataDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if mkErr := os.MkdirAll(cfg.DataDir, 0o600); mkErr != nil {
				slog.Error("failed to create data directory", "dir", cfg.DataDir, "err", mkErr)
				os.Exit(3)
			}
		} else {
			slog.Error("stat data directory", "dir", cfg.DataDir, "err", err)
			os.Exit(3)
		}
	} else if !st.IsDir() {
		slog.Error("data path not directory", "dir", cfg.DataDir)
		os.Exit(3)
	}

	// open / migrate sqlite database (file inside data dir)
	dbPath := filepath.Join(cfg.DataDir, "gone.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		slog.Error("open sqlite driver", "err", err)
		os.Exit(4)
	}
	defer db.Close()
	idx, err := sqlite.New(db)
	if err != nil {
		slog.Error("init sqlite schema", "err", err)
		os.Exit(4)
	}

	// filesystem blob storage lives under data dir/blobs
	blobDir := filepath.Join(cfg.DataDir, "blobs")
	if err := os.MkdirAll(blobDir, 0o600); err != nil {
		slog.Error("create blobs dir", "err", err)
		os.Exit(5)
	}
	blobs, err := filesystem.New(blobDir)
	if err != nil {
		slog.Error("init blob storage", "err", err)
		os.Exit(5)
	}

	// clock impl (inline real clock)
	clock := realClock{}

	// compose store
	st := store.New(idx, blobs, clock, 1024*4) // inline threshold fixed for now

	// app service
	svc := &app.Service{
		Store:    st,
		Clock:    clock,
		MaxBytes: cfg.MaxBytes,
		MinTTL:   cfg.MinTTL,
		MaxTTL:   cfg.MaxTTL,
	}

	// parse embedded templates (partials + index + about + secret)
	partialsBytes, err := fs.ReadFile(wembed.FS, "partials.tmpl.html")
	if err != nil {
		slog.Error("load partials template", "err", err)
		os.Exit(6)
	}
	indexBytes, err := fs.ReadFile(wembed.FS, "index.tmpl.html")
	if err != nil {
		slog.Error("load index template", "err", err)
		os.Exit(6)
	}
	indexTmpl, err := template.New("partials").Parse(string(partialsBytes))
	if err == nil {
		indexTmpl, err = indexTmpl.New("index").Parse(string(indexBytes))
	}
	if err != nil {
		slog.Error("parse index template", "err", err)
		os.Exit(6)
	}
	aboutBytes, err := fs.ReadFile(wembed.FS, "about.tmpl.html")
	if err != nil {
		slog.Error("load about template", "err", err)
		os.Exit(6)
	}
	aboutTmpl, err := template.New("partials").Parse(string(partialsBytes))
	if err == nil {
		aboutTmpl, err = aboutTmpl.New("about").Parse(string(aboutBytes))
	}
	if err != nil {
		slog.Error("parse about template", "err", err)
		os.Exit(6)
	}
	secretBytes, err := fs.ReadFile(wembed.FS, "secret.tmpl.html")
	if err != nil {
		slog.Error("load secret template", "err", err)
		os.Exit(6)
	}
	secretTmpl, err := template.New("partials").Parse(string(partialsBytes))
	if err == nil {
		secretTmpl, err = secretTmpl.New("secret").Parse(string(secretBytes))
	}
	if err != nil {
		slog.Error("parse secret template", "err", err)
		os.Exit(6)
	}

	// prepare assets fs (sub FS filtered for css/js served under /static)
	assets := http.FS(wembed.FS)

	// readiness probe: simple DB ping + list blob dir
	readiness := func(ctx context.Context) error {
		// simple ping via a lightweight query
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		if _, err := os.ReadDir(blobDir); err != nil {
			return err
		}
		return nil
	}

	handler := httpx.New(svc, cfg.MaxBytes, readiness)
	handler.IndexTmpl = httpx.TemplateRenderer{T: indexTmpl}
	handler.AboutTmpl = httpx.AboutTemplateRenderer{T: aboutTmpl}
	handler.SecretTmpl = httpx.TemplateRenderer{T: secretTmpl}
	handler.Assets = assets
	handler.MinTTL = cfg.MinTTL
	handler.MaxTTL = cfg.MaxTTL
	handler.TTLOptions = cfg.TTLOptions

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler.Router(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("starting server", "addr", cfg.Addr, "pid", os.Getpid())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
