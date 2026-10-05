package store_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

// storeTestManageHash is the manage hash used for every saved secret.
const storeTestManageHash = "4d616e6167652d68617368"

// storeFixedClock implements app.Clock for deterministic tests.
type storeFixedClock struct{ now time.Time }

// Now returns the clock's fixed timestamp.
//
// Parameters: none.
// Returns: the deterministic time configured on the receiver.
func (f storeFixedClock) Now() time.Time { return f.now }

// storeOpenTestDB opens a temporary SQLite database for store integration tests.
//
// Parameters:
//   - t: test handle used for cleanup and failure reporting.
//
// Returns: an open SQLite database handle configured with WAL pragmas.
func storeOpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "store.db") + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	return db
}

// storeWriteTempBlob writes a blob file directly for orphan cleanup tests.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: directory where the blob file should be written.
//   - id: secret id used to derive the blob file name.
//   - data: blob contents to persist.
//
// Returns: none; failures abort the test.
func storeWriteTempBlob(t *testing.T, dir, id string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, id+".blob")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}
}

// storeBytesReader returns a simple reader over b without copying.
//
// Parameters:
//   - b: bytes to expose through the returned reader.
//
// Returns: an io.Reader over b.
func storeBytesReader(b []byte) io.Reader { return &storeSliceReader{b: b} }

type storeSliceReader struct{ b []byte }

// Read copies bytes from the receiver into p.
//
// Parameters:
//   - p: destination byte slice.
//
// Returns: the number of bytes copied and io.EOF when exhausted.
func (r *storeSliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

var errStoreBlobOpen = errors.New("blob open failed")

type storeFlakyOpenBlobStore struct {
	data     []byte
	failOpen bool
	deleted  bool
}

// Write records the supplied blob data in memory.
//
// Parameters:
//   - id: ignored secret id.
//   - r: source reader.
//   - size: ignored expected size.
//
// Returns: any read error from r.
func (f *storeFlakyOpenBlobStore) Write(_ string, r io.Reader, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.data = data
	return nil
}

// Open opens the recorded blob or returns the configured failure.
//
// Parameters:
//   - id: ignored secret id.
//
// Returns: a reader for the recorded data, or errStoreBlobOpen when configured.
func (f *storeFlakyOpenBlobStore) Open(_ string) (io.ReadCloser, error) {
	if f.failOpen {
		return nil, errStoreBlobOpen
	}
	return io.NopCloser(storeBytesReader(f.data)), nil
}

// Delete records that blob deletion was requested.
//
// Parameters:
//   - id: ignored secret id.
//
// Returns: nil.
func (f *storeFlakyOpenBlobStore) Delete(_ string) error {
	f.deleted = true
	return nil
}

// List returns no blob ids for the flaky store.
//
// Parameters: none.
// Returns: an empty id list and nil error.
func (f *storeFlakyOpenBlobStore) List() ([]string, error) { return nil, nil }

// storeMockBlobStore is a minimal BlobStorage implementation for store tests.
type storeMockBlobStore struct {
	data      []byte
	deleteIDs []string
	listIDs   []string
	listErr   error
	deleteErr error
}

// Write records all bytes read from r.
//
// Parameters:
//   - id: ignored secret id.
//   - r: source reader.
//   - size: ignored expected size.
//
// Returns: any read error from r.
func (m *storeMockBlobStore) Write(_ string, r io.Reader, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.data = data
	return nil
}

// Open returns a reader over the recorded blob data.
//
// Parameters:
//   - id: ignored secret id.
//
// Returns: a read closer over the recorded data and nil error.
func (m *storeMockBlobStore) Open(_ string) (io.ReadCloser, error) {
	return io.NopCloser(storeBytesReader(m.data)), nil
}

// Delete records the requested id and returns the configured error.
//
// Parameters:
//   - id: secret id requested for deletion.
//
// Returns: the configured delete error.
func (m *storeMockBlobStore) Delete(id string) error {
	m.deleteIDs = append(m.deleteIDs, id)
	return m.deleteErr
}

// List returns configured blob ids or a configured error.
//
// Parameters: none.
// Returns: blob ids or a list error.
func (m *storeMockBlobStore) List() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listIDs, nil
}

// storeMockIndex is a configurable Index implementation for store tests.
type storeMockIndex struct {
	status         app.SecretStatus
	statusErr      error
	revokeExternal bool
	revokeErr      error
	claimResult    *store.IndexResult
	claimErr       error
	ackExternal    bool
	ackErr         error
	expired        []store.ExpiredRecord
	listIDs        []string
	listErr        error
}

// Insert accepts a row without persisting it.
//
// Parameters:
//   - ctx: ignored context.
//   - row: ignored new row.
//
// Returns: nil.
func (m storeMockIndex) Insert(_ context.Context, _ store.NewRow) error { return nil }
