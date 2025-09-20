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
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/haukened/gone/internal/config"
)

func main() {
	// Load and validate configuration.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(2)
	}

	// ensure the data directory exists
	if _, err := os.Stat(cfg.DataDir); os.IsNotExist(err) {
		slog.Info("data directory does not exist, creating", "dir", cfg.DataDir)
		if err := os.MkdirAll(cfg.DataDir, 0o600); err != nil {
			slog.Error("failed to create data directory", "dir", cfg.DataDir, "err", err)
			os.Exit(3)
		}
	}

	// create the HTTP mux and register the health endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// configure the HTTP server
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// start the HTTP server
	slog.Info("starting server", "addr", cfg.Addr, "pid", os.Getpid())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
