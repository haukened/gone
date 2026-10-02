package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/store"
	wembed "github.com/haukened/gone/web"
)

// freeAddr returns a loopback address with a currently unused port.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// holdAddr returns a loopback address whose port stays bound for the test.
func holdAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().String()
}

// waitFor polls url until it answers with want or the deadline passes.
func waitFor(t *testing.T, url string, header http.Header, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header = header
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not return %d in time", url, want)
}

// syncBuffer is a goroutine-safe bytes.Buffer for capturing logs.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the buffer.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the buffered contents.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs redirects the default slog logger for the duration of a test.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// testStorage opens a real database and blob directory under a temp dir.
func testStorage(t *testing.T) (*sql.DB, store.Index, string) {
	t.Helper()
	dir, blobDir, err := ensureDataDir(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("ensureDataDir: %v", err)
	}
	db, idx, err := openDatabase(dir)
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, idx, blobDir
}

// TestReadinessCheck covers healthy, database-down, and blob-dir-missing cases.
func TestReadinessCheck(t *testing.T) {
	db, _, blobDir := testStorage(t)
	check := readinessCheck(db, blobDir)
	if err := check(context.Background()); err != nil {
		t.Fatalf("healthy check: %v", err)
	}
	if err := os.Remove(blobDir); err != nil {
		t.Fatal(err)
	}
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error with blob dir missing")
	}
	_ = db.Close()
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error with database closed")
	}
}

// TestBuildHandler_NilErrorPage verifies the handler works without an error
// template and reports readiness.
func TestBuildHandler_NilErrorPage(t *testing.T) {
	db, idx, blobDir := testStorage(t)
	cfg := &config.Config{MaxBytes: 1024, MinTTL: time.Minute, MaxTTL: time.Hour}
	tmpls := &templates{
		index:  template.Must(template.New("index").Parse("i")),
		about:  template.Must(template.New("about").Parse("a")),
		secret: template.Must(template.New("secret").Parse("s")),
	}
	h := buildHandler(cfg, buildService(idx, stubBlobStorage{}, cfg, realClock{}), db, blobDir, tmpls, wembed.Assets).Router()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("readyz status = %d", rr.Code)
	}
}

// TestListenAndServe covers listener failure and graceful cancellation.
func TestListenAndServe(t *testing.T) {
	busy := &http.Server{Addr: holdAddr(t), Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	if err := listenAndServe(context.Background(), busy); err == nil {
		t.Fatal("expected error for address in use")
	}

	addr := freeAddr(t)
	srv := &http.Server{Addr: addr, Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listenAndServe(ctx, srv) }()
	waitFor(t, "http://"+addr+"/", nil, http.StatusNotFound)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("listenAndServe after cancel: %v", err)
	}
}

// TestListenAndServe_ClosedReturnsNil verifies an externally closed server
// is treated as a clean stop.
func TestListenAndServe_ClosedReturnsNil(t *testing.T) {
	addr := freeAddr(t)
	srv := &http.Server{Addr: addr, Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- listenAndServe(context.Background(), srv) }()
	waitFor(t, "http://"+addr+"/", nil, http.StatusNotFound)
	if err := shutdownServer(srv); err != nil {
		t.Fatalf("shutdownServer: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("listenAndServe after shutdown: %v", err)
	}
}

// TestStartMetrics covers disabled, token-less, enabled, and schema-failure
// configurations.
func TestStartMetrics(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		token string
	}{
		{"disabled", "", ""},
		{"address without token", freeAddr(t), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, _ := testStorage(t)
			mgr, err := startMetrics(context.Background(), db, &config.Config{MetricsAddr: tt.addr, MetricsToken: tt.token})
			if err != nil {
				t.Fatalf("startMetrics: %v", err)
			}
			mgr.Stop(context.Background())
		})
	}

	t.Run("enabled", func(t *testing.T) {
		db, _, _ := testStorage(t)
		addr := freeAddr(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mgr, err := startMetrics(ctx, db, &config.Config{MetricsAddr: addr, MetricsToken: "tok"})
		if err != nil {
			t.Fatalf("startMetrics: %v", err)
		}
		defer mgr.Stop(context.Background())
		waitFor(t, "http://"+addr+"/", http.Header{"Authorization": {"Bearer tok"}}, http.StatusOK)
	})

	t.Run("listener failure is logged not returned", func(t *testing.T) {
		logs := captureLogs(t)
		db, _, _ := testStorage(t)
		mgr, err := startMetrics(context.Background(), db, &config.Config{MetricsAddr: holdAddr(t), MetricsToken: "tok"})
		if err != nil {
			t.Fatalf("startMetrics: %v", err)
		}
		defer mgr.Stop(context.Background())
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(logs.String(), "metrics server error") {
			if time.Now().After(deadline) {
				t.Fatal("expected metrics server error to be logged")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("schema failure", func(t *testing.T) {
		db, _, _ := testStorage(t)
		_ = db.Close()
		if _, err := startMetrics(context.Background(), db, &config.Config{}); err == nil {
			t.Fatal("expected schema init error on closed database")
		}
	})
}

// TestServe_SetupErrors covers blob storage and template failures.
func TestServe_SetupErrors(t *testing.T) {
	db, idx, blobDir := testStorage(t)
	tests := []struct {
		name    string
		blobDir string
		assets  fstest.MapFS
		want    string
	}{
		{"missing blob dir", filepath.Join(blobDir, "missing"), nil, "init blob storage"},
		{"missing templates", blobDir, fstest.MapFS{}, "partials.tmpl.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &components{cfg: &config.Config{}, db: db, idx: idx, blobDir: tt.blobDir, assets: tt.assets}
			err := c.serve(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestShutdownServer_Unstarted verifies shutting down a never-started server
// is harmless.
func TestShutdownServer_Unstarted(t *testing.T) {
	if err := shutdownServer(&http.Server{ReadHeaderTimeout: time.Second}); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("shutdownServer on unstarted server: %v", err)
	}
}
