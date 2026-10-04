package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setEnv configures run via GONE_* environment variables for one test.
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

// TestRun_StartupErrors covers configuration, data-dir, and listener failures.
func TestRun_StartupErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	badDB := t.TempDir()
	if err := os.Mkdir(filepath.Join(badDB, "gone.db"), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"invalid addr", map[string]string{"GONE_ADDR": "not-an-addr", "GONE_DATA_DIR": t.TempDir()}, "load config"},
		{"data dir is a file", map[string]string{"GONE_ADDR": freeAddr(t), "GONE_DATA_DIR": file}, "data dir"},
		{"database is a directory", map[string]string{"GONE_ADDR": freeAddr(t), "GONE_DATA_DIR": badDB}, "sqlite"},
		{"address in use", map[string]string{"GONE_ADDR": holdAddr(t), "GONE_DATA_DIR": t.TempDir()}, "address already in use"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)
			err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestRun_ServesUntilCanceled starts the full service with metrics, checks
// both listeners, and verifies cancellation shuts down cleanly.
func TestRun_ServesUntilCanceled(t *testing.T) {
	addr, metricsAddr := freeAddr(t), freeAddr(t)
	setEnv(t, map[string]string{
		"GONE_ADDR":          addr,
		"GONE_DATA_DIR":      filepath.Join(t.TempDir(), "data"),
		"GONE_METRICS_ADDR":  metricsAddr,
		"GONE_METRICS_TOKEN": "tok",
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()

	waitFor(t, "http://"+addr+"/healthz", nil, http.StatusOK)
	waitFor(t, "http://"+addr+"/readyz", nil, http.StatusOK)
	waitFor(t, "http://"+metricsAddr+"/", http.Header{"Authorization": {"Bearer tok"}}, http.StatusOK)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run after cancel: %v", err)
	}
}
