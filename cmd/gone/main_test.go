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
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// stubIndex implements store.Index minimally for buildService test.
type stubIndex struct{}

func (stubIndex) Insert(context.Context, string, app.Meta, []byte, bool, int64, time.Time, time.Time) error {
	return nil
}
func (stubIndex) Claim(context.Context, string, string, bool, time.Time, time.Time, store.ExternalOpener) (*store.IndexResult, error) {
	return nil, os.ErrNotExist
}
func (stubIndex) Ack(context.Context, string, string) (bool, error) { return false, os.ErrNotExist }
func (stubIndex) DeleteExpired(context.Context, time.Time) ([]store.ExpiredRecord, error) {
	return nil, nil
}
func (stubIndex) ListExternalIDs(context.Context) ([]string, error) { return nil, nil }

// stubBlobStorage implements store.BlobStorage.
type stubBlobStorage struct{}

func (stubBlobStorage) Write(string, io.Reader, int64) error { return nil }
func (stubBlobStorage) Open(string) (io.ReadCloser, error)   { return nil, os.ErrNotExist }
func (stubBlobStorage) Delete(string) error                  { return nil }
func (stubBlobStorage) List() ([]string, error)              { return nil, nil }

type recordingIndex struct {
	inline   []byte
	external bool
}

func (r *recordingIndex) Insert(_ context.Context, _ string, _ app.Meta, inline []byte, external bool, _ int64, _, _ time.Time) error {
	r.inline = inline
	r.external = external
	return nil
}
func (r *recordingIndex) Claim(context.Context, string, string, bool, time.Time, time.Time, store.ExternalOpener) (*store.IndexResult, error) {
	return nil, os.ErrNotExist
}
func (r *recordingIndex) Ack(context.Context, string, string) (bool, error) {
	return false, os.ErrNotExist
}
func (r *recordingIndex) DeleteExpired(context.Context, time.Time) ([]store.ExpiredRecord, error) {
	return nil, nil
}
func (r *recordingIndex) ListExternalIDs(context.Context) ([]string, error) { return nil, nil }

type recordingBlobStorage struct {
	wrote bool
	data  []byte
}

func (r *recordingBlobStorage) Write(_ string, src io.Reader, _ int64) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	r.wrote = true
	r.data = data
	return nil
}
func (r *recordingBlobStorage) Open(string) (io.ReadCloser, error) { return nil, os.ErrNotExist }
func (r *recordingBlobStorage) Delete(string) error                { return nil }
func (r *recordingBlobStorage) List() ([]string, error)            { return nil, nil }

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
	cfg := &config.Config{InlineMaxBytes: 64, MaxBytes: 1234, MinTTL: time.Minute, MaxTTL: 2 * time.Minute, ClaimLease: 75 * time.Second}
	// Build service using stub index/blob implementations by wrapping underlying store.New expectations.
	s := buildService(stubIndex{}, stubBlobStorage{}, cfg, realClock{})
	if s.MaxBytes != 1234 {
		t.Fatalf("MaxBytes mismatch got %d", s.MaxBytes)
	}
	if s.MinTTL != time.Minute || s.MaxTTL != 2*time.Minute {
		t.Fatalf("TTL mismatch")
	}
	if s.ClaimLease != 75*time.Second {
		t.Fatalf("ClaimLease mismatch got %v", s.ClaimLease)
	}
}

func TestBuildServiceUsesConfiguredInlineThreshold(t *testing.T) {
	idx := &recordingIndex{}
	blobs := &recordingBlobStorage{}
	cfg := &config.Config{InlineMaxBytes: 4, MaxBytes: 32, MinTTL: time.Minute, MaxTTL: 2 * time.Minute}
	s := buildService(idx, blobs, cfg, realClock{})

	if _, _, err := s.CreateSecret(context.Background(), strings.NewReader("external"), int64(len("external")), 1, "nonce", time.Minute); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if !blobs.wrote {
		t.Fatalf("expected payload to be written to blob storage")
	}
	if !idx.external {
		t.Fatalf("expected index insert to mark secret external")
	}
	if len(idx.inline) != 0 {
		t.Fatalf("expected no inline payload, got %d bytes", len(idx.inline))
	}
}

// TestNewServer ensures timeouts and addr applied.
func TestNewServer(t *testing.T) {
	cfg := &config.Config{Addr: ":9999"}
	srv := newServer(cfg, http.NewServeMux())
	if srv.Addr != ":9999" {
		t.Fatalf("addr mismatch got %s", srv.Addr)
	}
	if srv.ReadTimeout == 0 || srv.WriteTimeout == 0 || srv.ReadHeaderTimeout == 0 {
		t.Fatalf("expected non-zero timeouts")
	}
	if srv.ReadHeaderTimeout >= srv.ReadTimeout {
		t.Fatalf("expected header timeout (%v) tighter than body timeout (%v)", srv.ReadHeaderTimeout, srv.ReadTimeout)
	}
}

// TestBuildHandler exercises basic route wiring for index template.
func TestBuildHandler_IndexRoute(t *testing.T) {
	// Prepare temp DB for sqlite index.
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gone.db")
	db, err := sql.Open("sqlite", dbPath)
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

// TestEnsureDataDir_EnforcesPrivatePerms verifies the data and blobs
// directories end up owner-only (0o700), whether newly created or pre-existing
// with looser permissions.
func TestEnsureDataDir_EnforcesPrivatePerms(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{name: "new", setup: func(*testing.T, string) {}},
		{name: "existing loose data dir", setup: func(t *testing.T, dir string) {
			mkdirMode(t, dir, 0o777)
		}},
		{name: "existing loose blobs dir", setup: func(t *testing.T, dir string) {
			mkdirMode(t, dir, 0o755)
			mkdirMode(t, filepath.Join(dir, "blobs"), 0o777)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			tc.setup(t, dir)
			dataDir, blobDir, err := ensureDataDir(dir)
			if err != nil {
				t.Fatalf("ensureDataDir: %v", err)
			}
			for _, p := range []string{dataDir, blobDir} {
				st, err := os.Stat(p)
				if err != nil {
					t.Fatalf("stat %s: %v", p, err)
				}
				if got := st.Mode().Perm(); got != privateDirPerm {
					t.Fatalf("%s perm got %o want %o", p, got, privateDirPerm)
				}
			}
		})
	}
}

// mkdirMode creates dir and forces mode regardless of the process umask.
func mkdirMode(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(dir, mode); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
}

// Failure path: ensureDataDir where blobs path exists as a file.
func TestEnsureDataDir_BlobsFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blobs"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := ensureDataDir(dir); err == nil {
		t.Fatalf("expected error when blobs is a file")
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

// TestOpenDatabase_AppliesHardenedPragmas verifies the hardened DSN pragmas are
// applied to connections returned by openDatabase.
func TestOpenDatabase_AppliesHardenedPragmas(t *testing.T) {
	db, _, err := openDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tests := []struct {
		pragma string
		want   string
	}{
		{pragma: "journal_mode", want: "wal"},
		{pragma: "foreign_keys", want: "1"},
		{pragma: "busy_timeout", want: "5000"},
		{pragma: "synchronous", want: "2"}, // FULL
	}
	for _, tt := range tests {
		t.Run(tt.pragma, func(t *testing.T) {
			var got string
			if err := db.QueryRow("PRAGMA " + tt.pragma).Scan(&got); err != nil {
				t.Fatalf("query pragma: %v", err)
			}
			if !strings.EqualFold(got, tt.want) {
				t.Fatalf("PRAGMA %s = %q, want %q", tt.pragma, got, tt.want)
			}
		})
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
