package main

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/store/sqlite"
	wembed "github.com/haukened/gone/v3/web"
)

// TestBuildHandler_IndexRoute exercises basic route wiring for index template.
//
// Parameters:
//   - t: the test handle.
func TestBuildHandler_IndexRoute(t *testing.T) {
	tmp := t.TempDir()
	db := openTestDB(t, filepath.Join(tmp, "gone.db"))
	idx, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite init: %v", err)
	}
	blobDir := filepath.Join(tmp, "blobs")
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		t.Fatalf("mkdir blobs: %v", err)
	}

	cfg := &config.Config{MaxBytes: 2048, MinTTL: time.Minute, MaxTTL: 2 * time.Minute, TTLOptions: []domain.TTLOption{{Duration: time.Minute, Label: "1m"}}}
	svc := buildService(idx, stubBlobStorage{}, cfg, realClock{})
	h := buildHandler(cfg, svc, db, blobDir, minimalTemplates(), wembed.Assets).Router()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("index status got %d", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("expected body content")
	}
}

// openTestDB opens a SQLite database and registers cleanup.
//
// Parameters:
//   - t: the test handle.
//   - path: the database path.
//
// Returns:
//   - *sql.DB: the opened database.
func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// minimalTemplates builds the smallest complete template set for route tests.
//
// Returns:
//   - *templates: a complete template bundle.
func minimalTemplates() *templates {
	return &templates{
		index:     template.Must(template.New("index").Parse("<html>index</html>")),
		about:     template.Must(template.New("about").Parse("about")),
		secret:    template.Must(template.New("secret").Parse("secret")),
		manage:    template.Must(template.New("manage").Parse("manage")),
		errorPage: template.Must(template.New("error").Parse("error")),
	}
}
