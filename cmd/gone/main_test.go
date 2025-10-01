package main

import (
	"context"
	"database/sql"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

// stubIndex implements store.Index minimally for buildService test.
type stubIndex struct{}

func (stubIndex) Insert(context.Context, string, app.Meta, []byte, bool, int64, time.Time, time.Time) error {
	return nil
}
func (stubIndex) Consume(context.Context, string, time.Time) (*store.IndexResult, error) {
	return nil, os.ErrNotExist
}
func (stubIndex) DeleteExpired(context.Context, time.Time) ([]store.ExpiredRecord, error) {
	return nil, nil
}
func (stubIndex) ListExternalIDs(context.Context) ([]string, error) { return nil, nil }

// stubBlobStorage implements store.BlobStorage.
type stubBlobStorage struct{}

func (stubBlobStorage) Write(string, io.Reader, int64) error  { return nil }
func (stubBlobStorage) Consume(string) (io.ReadCloser, error) { return nil, os.ErrNotExist }
func (stubBlobStorage) Delete(string) error                   { return nil }
func (stubBlobStorage) List() ([]string, error)               { return nil, nil }

// TestEnsureDataDir verifies directory and blob subdirectory creation.
func TestEnsureDataDir(t *testing.T) {
	tmp := t.TempDir()
	data := filepath.Join(tmp, "data-root")
	gotData, gotBlob, err := ensureDataDir(data)
	if err != nil {
		t.Fatalf("ensureDataDir error: %v", err)
	}
	if gotData != data {
		t.Fatalf("data dir mismatch got %s want %s", gotData, data)
	}
	if gotBlob != filepath.Join(data, "blobs") {
		t.Fatalf("blob dir mismatch got %s", gotBlob)
	}
	if _, err := os.Stat(gotData); err != nil {
		t.Fatalf("data dir stat: %v", err)
	}
	if _, err := os.Stat(gotBlob); err != nil {
		t.Fatalf("blob dir stat: %v", err)
	}
}

// TestParseAllTemplates ensures embedded templates can be loaded.
func TestLoadTemplates(t *testing.T) {
	tmpls, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates error: %v", err)
	}
	if tmpls.index == nil || tmpls.about == nil || tmpls.secret == nil || tmpls.errorPage == nil {
		t.Fatalf("expected all templates non-nil")
	}
}

// TestBuildService validates service field propagation.
func TestBuildService(t *testing.T) {
	cfg := &config.Config{MaxBytes: 1234, MinTTL: time.Minute, MaxTTL: 2 * time.Minute}
	// Build service using stub index/blob implementations by wrapping underlying store.New expectations.
	s := buildService(stubIndex{}, stubBlobStorage{}, cfg, realClock{})
	if s.MaxBytes != 1234 {
		t.Fatalf("MaxBytes mismatch got %d", s.MaxBytes)
	}
	if s.MinTTL != time.Minute || s.MaxTTL != 2*time.Minute {
		t.Fatalf("TTL mismatch")
	}
}

// TestNewServer ensures timeouts and addr applied.
func TestNewServer(t *testing.T) {
	cfg := &config.Config{Addr: ":9999"}
	srv := newServer(cfg, http.NewServeMux())
	if srv.Addr != ":9999" {
		t.Fatalf("addr mismatch got %s", srv.Addr)
	}
	if srv.ReadTimeout == 0 || srv.WriteTimeout == 0 {
		t.Fatalf("expected non-zero timeouts")
	}
}

// TestBuildHandler exercises basic route wiring for index template.
func TestBuildHandler_IndexRoute(t *testing.T) {
	// Prepare temp DB for sqlite index.
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gone.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	idx, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite init: %v", err)
	}
	blobDir := filepath.Join(tmp, "blobs")
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		t.Fatalf("mkdir blobs: %v", err)
	}
	// Minimal templates
	tmpls := &templates{
		index:     template.Must(template.New("index").Parse("<html>index</html>")),
		about:     template.Must(template.New("about").Parse("about")),
		secret:    template.Must(template.New("secret").Parse("secret")),
		errorPage: template.Must(template.New("error").Parse("error")),
	}
	cfg := &config.Config{MaxBytes: 2048, MinTTL: time.Minute, MaxTTL: 2 * time.Minute, TTLOptions: []domain.TTLOption{{Duration: time.Minute, Label: "1m"}}}
	svc := buildService(idx, stubBlobStorage{}, cfg, realClock{})
	h := buildHandler(cfg, svc, db, blobDir, tmpls)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("index status got %d", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("expected body content")
	}
}

// Failure path: ensureDataDir where path exists as file.
func TestEnsureDataDir_FilePathError(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := ensureDataDir(filePath); err == nil {
		t.Fatalf("expected error for file path")
	}
}

// Failure path: openDatabase with directory lacking permissions (simulate by using dir path as file).
func TestOpenDatabase_Error(t *testing.T) {
	tmp := t.TempDir()
	// Use a sub directory we make read-only to trigger open error by removing write perms after creation.
	dir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dir, 0o500); err != nil { // no write bit
		t.Fatalf("mkdir: %v", err)
	}
	// Make file path unwritable by using a directory with no write; sqlite should fail create db file.
	if _, _, err := openDatabase(dir); err == nil {
		t.Fatalf("expected openDatabase error")
	}
}

// Failure path: loadTemplatesFrom missing partials or page templates.
func TestLoadTemplatesFrom_Error(t *testing.T) {
	// Provide FS missing partials.tmpl.html so initial read fails.
	fsys := fstest.MapFS{}
	if _, err := loadTemplatesFrom(fsys); err == nil {
		t.Fatalf("expected error due to missing partials template")
	}
}
