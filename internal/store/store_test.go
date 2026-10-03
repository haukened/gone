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

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/filesystem"
	"github.com/haukened/gone/internal/store/sqlite"
)

// testManageHash is the manage hash used for every saved secret.
const testManageHash = "4d616e6167652d68617368"

// fixedClock implements app.Clock for deterministic tests.
type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

// openTestDB mirrors the sqlite test helper.
func openTestDB(t *testing.T) *sql.DB {
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

// writeTempBlob writes a blob directly for orphan cleanup tests.
func writeTempBlob(t *testing.T, dir, id string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, id+".blob")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}
}

func TestStoreSaveInlineClaimAndAck(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	bs, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	st := store.New(ix, bs, clk, 64)

	id := "11111111111111111111111111111111"
	meta := app.Meta{Version: 1, NonceB64u: "nonceA"}
	data := []byte("hello-inline")
	expires := now.Add(5 * time.Minute)
	lease := now.Add(time.Minute)
	if err := st.Save(ctx, id, meta, testManageHash, io.NopCloser(bytesReader(data)), int64(len(data)), expires); err != nil {
		t.Fatalf("Save inline: %v", err)
	}
	claimed, err := st.Claim(ctx, id, "claim-hash-a", false, lease)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	b, err := io.ReadAll(claimed.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := claimed.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(b) != string(data) {
		t.Fatalf("data mismatch got=%q", b)
	}
	if claimed.Size != int64(len(data)) {
		t.Fatalf("size mismatch got=%d", claimed.Size)
	}
	if claimed.Meta.Version != meta.Version || claimed.Meta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
	if claimed.ClaimedUntil.Unix() != lease.Unix() {
		t.Fatalf("lease mismatch got=%v want=%v", claimed.ClaimedUntil, lease)
	}
	if _, err = st.Claim(ctx, id, "other-hash", false, lease); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for second fresh claim, got %v", err)
	}
	if err := st.Ack(ctx, id, "claim-hash-a"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if _, err = st.Claim(ctx, id, "claim-hash-a", true, lease); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after ack, got %v", err)
	}
}

func TestStoreSaveExternalClaimAndAckDeletesBlob(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	blobDir := t.TempDir()
	bs, err := filesystem.New(blobDir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	st := store.New(ix, bs, clk, 4)

	id := "22222222222222222222222222222222"
	meta := app.Meta{Version: 2, NonceB64u: "nonceB"}
	data := []byte("this-is-external-data")
	expires := now.Add(10 * time.Minute)
	lease := now.Add(time.Minute)
	if err := st.Save(ctx, id, meta, testManageHash, io.NopCloser(bytesReader(data)), int64(len(data)), expires); err != nil {
		t.Fatalf("Save external: %v", err)
	}
	if _, err := os.Stat(filepath.Join(blobDir, id+".blob")); err != nil {
		t.Fatalf("expected blob file: %v", err)
	}
	claimed, err := st.Claim(ctx, id, "claim-hash-b", false, lease)
	if err != nil {
		t.Fatalf("Claim external: %v", err)
	}
	readData, err := io.ReadAll(claimed.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := claimed.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(readData) != string(data) {
		t.Fatalf("payload mismatch")
	}
	if claimed.Size != int64(len(data)) {
		t.Fatalf("size mismatch")
	}
	if claimed.Meta.Version != meta.Version || claimed.Meta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
	if _, err := os.Stat(filepath.Join(blobDir, id+".blob")); err != nil {
		t.Fatalf("expected blob to remain after claim reader close: %v", err)
	}
	if err := st.Ack(ctx, id, "claim-hash-b"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(blobDir, id+".blob")); !os.IsNotExist(err) {
		t.Fatalf("expected blob removed by ack, err=%v", err)
	}
}

func TestStoreClaimExternalOpenFailureIsRetryable(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	blobs := &flakyOpenBlobStore{failOpen: true}
	st := store.New(ix, blobs, clk, 4)

	id := "22222222222222222222222222222223"
	data := []byte("this-is-external-data")
	lease := now.Add(time.Minute)
	if err := st.Save(ctx, id, app.Meta{Version: 2, NonceB64u: "nonceB"}, testManageHash, io.NopCloser(bytesReader(data)), int64(len(data)), now.Add(10*time.Minute)); err != nil {
		t.Fatalf("Save external: %v", err)
	}
	if _, err := st.Claim(ctx, id, "claim-hash-c", false, lease); !errors.Is(err, errBlobOpen) {
		t.Fatalf("expected blob open error, got %v", err)
	}
	blobs.failOpen = false
	claimed, err := st.Claim(ctx, id, "claim-hash-c", false, lease)
	if err != nil {
		t.Fatalf("retry claim: %v", err)
	}
	readData, err := io.ReadAll(claimed.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := claimed.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(readData) != string(data) {
		t.Fatalf("payload mismatch")
	}
	if claimed.Size != int64(len(data)) {
		t.Fatalf("size mismatch")
	}
	if claimed.Meta.NonceB64u != "nonceB" {
		t.Fatalf("meta mismatch")
	}
	if blobs.deleted {
		t.Fatalf("did not expect blob deleted before ack")
	}
	if err := st.Ack(ctx, id, "claim-hash-c"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if !blobs.deleted {
		t.Fatalf("expected blob deleted after ack")
	}
}

func TestStoreClaimExternalMissingReader(t *testing.T) {
	st := store.New(mockIndex{claimResult: &store.IndexResult{External: true, Size: 7}}, &mockBlobStore{}, fixedClock{now: time.Now()}, 4)
	_, err := st.Claim(context.Background(), "id", "hash", false, time.Now().Add(time.Minute))
	if err == nil || err.Error() != "external reader missing" {
		t.Fatalf("expected missing external reader error, got %v", err)
	}
}

func TestStoreClaimExpired(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	bs, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	st := store.New(ix, bs, clk, 64)

	id := "33333333333333333333333333333333"
	data := []byte("x")
	expires := now.Add(-1 * time.Minute)
	if err := st.Save(ctx, id, app.Meta{Version: 1, NonceB64u: "nC"}, testManageHash, io.NopCloser(bytesReader(data)), int64(len(data)), expires); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := st.Claim(ctx, id, "claim-hash-d", false, now.Add(time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired claim, got %v", err)
	}
}

func TestStoreAckDeletesExternalBlobOnly(t *testing.T) {
	cases := []struct {
		name       string
		external   bool
		wantDelete int
	}{
		{name: "inline", external: false, wantDelete: 0},
		{name: "external", external: true, wantDelete: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := mockIndex{ackExternal: tc.external}
			blobs := &mockBlobStore{}
			st := store.New(idx, blobs, fixedClock{now: time.Now()}, 4)
			if err := st.Ack(context.Background(), "id", "hash"); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if got := len(blobs.deleteIDs); got != tc.wantDelete {
				t.Fatalf("delete count got=%d want=%d", got, tc.wantDelete)
			}
		})
	}
}

func TestStoreAckPropagatesIndexError(t *testing.T) {
	ackErr := errors.New("ack failed")
	st := store.New(mockIndex{ackErr: ackErr}, &mockBlobStore{}, fixedClock{now: time.Now()}, 4)
	if err := st.Ack(context.Background(), "id", "hash"); !errors.Is(err, ackErr) {
		t.Fatalf("expected ack error, got %v", err)
	}
}

func TestStoreRevokeDeletesExternalBlobOnly(t *testing.T) {
	cases := []struct {
		name       string
		external   bool
		wantDelete int
	}{
		{name: "inline", external: false, wantDelete: 0},
		{name: "external", external: true, wantDelete: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blobs := &mockBlobStore{}
			st := store.New(mockIndex{revokeExternal: tc.external}, blobs, fixedClock{now: time.Now()}, 4)
			if err := st.Revoke(context.Background(), "id", "hash"); err != nil {
				t.Fatalf("Revoke: %v", err)
			}
			if got := len(blobs.deleteIDs); got != tc.wantDelete {
				t.Fatalf("delete count got=%d want=%d", got, tc.wantDelete)
			}
		})
	}
}

func TestStoreRevokePropagatesIndexError(t *testing.T) {
	blobs := &mockBlobStore{}
	st := store.New(mockIndex{revokeErr: app.ErrNotFound, revokeExternal: true}, blobs, fixedClock{now: time.Now()}, 4)
	if err := st.Revoke(context.Background(), "id", "hash"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if len(blobs.deleteIDs) != 0 {
		t.Fatalf("blob deleted on failed revoke")
	}
}

func TestStoreStatusPassesThrough(t *testing.T) {
	want := app.SecretStatus{CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0)}
	st := store.New(mockIndex{status: want}, &mockBlobStore{}, fixedClock{now: time.Now()}, 4)
	got, err := st.Status(context.Background(), "id", "hash")
	if err != nil || got != want {
		t.Fatalf("Status = %+v, %v", got, err)
	}
}

func TestStoreRevokeExternalRemovesBlobFile(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	dir := t.TempDir()
	bs, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	st := store.New(ix, bs, fixedClock{now: now}, 4)
	id := "77777777777777777777777777777777"
	data := []byte("external-payload")
	if err = st.Save(ctx, id, app.Meta{Version: 1, NonceB64u: "n"}, testManageHash, bytesReader(data), int64(len(data)), now.Add(time.Hour)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err = st.Status(ctx, id, testManageHash); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err = st.Revoke(ctx, id, testManageHash); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("blob files remain after revoke: %v", ids)
	}
	if _, err = st.Status(ctx, id, testManageHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Status after revoke = %v", err)
	}
}

func TestStoreManageNilReceiver(t *testing.T) {
	var s *store.Store
	if _, err := s.Status(context.Background(), "id", "hash"); err == nil {
		t.Fatalf("expected error on nil store Status")
	}
	if err := s.Revoke(context.Background(), "id", "hash"); err == nil {
		t.Fatalf("expected error on nil store Revoke")
	}
}

func TestStoreDeleteExpired(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	blobDir := t.TempDir()
	bs, err := filesystem.New(blobDir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	st := store.New(ix, bs, clk, 4)

	if err := st.Save(ctx, "44444444444444444444444444444444", app.Meta{Version: 1, NonceB64u: "a"}, testManageHash, io.NopCloser(bytesReader([]byte("external-data"))), int64(len("external-data")), now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("save ext: %v", err)
	}
	if err := st.Save(ctx, "55555555555555555555555555555555", app.Meta{Version: 1, NonceB64u: "b"}, testManageHash, io.NopCloser(bytesReader([]byte("inl"))), 3, now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("save inl: %v", err)
	}
	if err := st.Save(ctx, "66666666666666666666666666666666", app.Meta{Version: 1, NonceB64u: "c"}, testManageHash, io.NopCloser(bytesReader([]byte("f"))), 1, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("save future: %v", err)
	}
	if _, err := os.Stat(filepath.Join(blobDir, "44444444444444444444444444444444.blob")); err != nil {
		t.Fatalf("missing ext blob: %v", err)
	}
	count, err := st.DeleteExpired(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 expired removed, got %d", count)
	}
	if _, err := os.Stat(filepath.Join(blobDir, "44444444444444444444444444444444.blob")); !os.IsNotExist(err) {
		t.Fatalf("expected external blob removed by janitor, err=%v", err)
	}
	if _, err := st.Claim(ctx, "66666666666666666666666666666666", "future-hash", false, now.Add(time.Minute)); err != nil {
		t.Fatalf("future claim: %v", err)
	}
}

func TestStoreDeleteExpiredCountsClaimMetrics(t *testing.T) {
	metrics := &countingMetrics{counts: map[string]int64{}}
	st := store.New(mockIndex{expired: []store.ExpiredRecord{
		{ID: "claimed-inline", Claimed: true},
		{ID: "claimed-external", External: true, Claimed: true},
		{ID: "ttl-external", External: true},
	}}, &mockBlobStore{}, fixedClock{now: time.Now()}, 4).WithMetrics(metrics)
	count, err := st.DeleteExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if count != 3 {
		t.Fatalf("count got=%d want=3", count)
	}
	if got := metrics.counts[store.CounterClaimsExpired]; got != 2 {
		t.Fatalf("claimed-expired metric got=%d want=2", got)
	}
}

func TestStoreReconcileDeletesOrphan(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := fixedClock{now: now}
	db := openTestDB(t)
	ix, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	blobDir := t.TempDir()
	bs, err := filesystem.New(blobDir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	st := store.New(ix, bs, clk, 4)

	writeTempBlob(t, blobDir, "77777777777777777777777777777777", []byte("zzz"))
	time.Sleep(1100 * time.Millisecond)
	if err := st.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(blobDir, "77777777777777777777777777777777.blob")); !os.IsNotExist(err) {
		t.Fatalf("expected orphan removed, err=%v", err)
	}
}

// bytesReader returns a simple reader over b without copying.
func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct{ b []byte }

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

var errBlobOpen = errors.New("blob open failed")

type flakyOpenBlobStore struct {
	data     []byte
	failOpen bool
	deleted  bool
}

func (f *flakyOpenBlobStore) Write(_ string, r io.Reader, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.data = data
	return nil
}

func (f *flakyOpenBlobStore) Open(_ string) (io.ReadCloser, error) {
	if f.failOpen {
		return nil, errBlobOpen
	}
	return io.NopCloser(bytesReader(f.data)), nil
}

func (f *flakyOpenBlobStore) Delete(_ string) error {
	f.deleted = true
	return nil
}

func (f *flakyOpenBlobStore) List() ([]string, error) { return nil, nil }

// --- Construction / nil guard tests ---

// mockBlobStore is a minimal BlobStorage implementation for store tests.
type mockBlobStore struct {
	data      []byte
	deleteIDs []string
	listIDs   []string
	listErr   error
	deleteErr error
}

func (m *mockBlobStore) Write(_ string, r io.Reader, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.data = data
	return nil
}

func (m *mockBlobStore) Open(_ string) (io.ReadCloser, error) {
	return io.NopCloser(bytesReader(m.data)), nil
}

func (m *mockBlobStore) Delete(id string) error {
	m.deleteIDs = append(m.deleteIDs, id)
	return m.deleteErr
}

func (m *mockBlobStore) List() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listIDs, nil
}

// mockIndex is a configurable Index implementation for store tests.
type mockIndex struct {
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

func (m mockIndex) Insert(_ context.Context, _ store.NewRow) error {
	return nil
}

func (m mockIndex) Status(_ context.Context, _, _ string, _ time.Time) (app.SecretStatus, error) {
	return m.status, m.statusErr
}

func (m mockIndex) Revoke(_ context.Context, _, _ string, _ time.Time) (bool, error) {
	return m.revokeExternal, m.revokeErr
}

func (m mockIndex) Claim(_ context.Context, _ string, _ string, _ bool, _ time.Time, _ time.Time, _ store.ExternalOpener) (*store.IndexResult, error) {
	if m.claimErr != nil {
		return nil, m.claimErr
	}
	if m.claimResult != nil {
		return m.claimResult, nil
	}
	return nil, app.ErrNotFound
}

func (m mockIndex) Ack(_ context.Context, _ string, _ string) (bool, error) {
	if m.ackErr != nil {
		return false, m.ackErr
	}
	return m.ackExternal, nil
}

func (m mockIndex) DeleteExpired(_ context.Context, _ time.Time) ([]store.ExpiredRecord, error) {
	return m.expired, nil
}

func (m mockIndex) ListExternalIDs(_ context.Context) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listIDs, nil
}

// countingMetrics records counter increments by name.
type countingMetrics struct{ counts map[string]int64 }

func (c *countingMetrics) Inc(name string, delta int64) { c.counts[name] += delta }

func TestStoreNilReceiverClaim(t *testing.T) {
	var s *store.Store
	if _, err := s.Claim(context.Background(), "any", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error on nil store Claim")
	}
}

func TestStoreNilReceiverAck(t *testing.T) {
	var s *store.Store
	if err := s.Ack(context.Background(), "any", "hash"); err == nil {
		t.Fatalf("expected error on nil store Ack")
	}
}

func TestStoreNilReceiverSave(t *testing.T) {
	var s *store.Store
	if err := s.Save(context.Background(), "id", app.Meta{}, testManageHash, bytesReader([]byte("a")), 1, time.Now()); err == nil {
		t.Fatalf("expected error on nil store Save")
	}
}

func TestStoreNilIndex(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	bs := &mockBlobStore{}
	s := store.New(nil, bs, clk, 10)
	if _, err := s.Claim(context.Background(), "x", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error with nil index")
	}
	if err := s.Ack(context.Background(), "x", "hash"); err == nil {
		t.Fatalf("expected ack error with nil index")
	}
}

func TestStoreNilBlobStorage(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	ix := mockIndex{}
	s := store.New(ix, nil, clk, 10)
	if _, err := s.Claim(context.Background(), "x", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error with nil blob storage")
	}
	if err := s.Ack(context.Background(), "x", "hash"); err == nil {
		t.Fatalf("expected ack error with nil blob storage")
	}
}

func TestStoreNilClock(t *testing.T) {
	ix := mockIndex{}
	bs := &mockBlobStore{}
	s := store.New(ix, bs, nil, 10)
	if err := s.Save(context.Background(), "x", app.Meta{}, testManageHash, bytesReader([]byte("a")), 1, time.Now()); err == nil {
		t.Fatalf("expected error with nil clock in Save")
	}
	if _, err := s.Claim(context.Background(), "x", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error with nil clock in Claim")
	}
}

func TestStoreSaveNegativeSize(t *testing.T) {
	ix := mockIndex{}
	bs := &mockBlobStore{}
	clk := fixedClock{now: time.Now()}
	s := store.New(ix, bs, clk, 10)
	if err := s.Save(context.Background(), "x", app.Meta{}, testManageHash, bytesReader([]byte("a")), -1, time.Now()); err == nil {
		t.Fatalf("expected error for negative size")
	}
}

// --- Reconcile error path tests ---

func TestStoreReconcileNilIndex(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	bs := &mockBlobStore{}
	s := store.New(nil, bs, clk, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected error with nil index in Reconcile")
	}
}

func TestStoreReconcileNilBlobs(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	ix := mockIndex{}
	s := store.New(ix, nil, clk, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected error with nil blob storage in Reconcile")
	}
}

func TestStoreReconcileBlobListError(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	ix := mockIndex{}
	bs := &mockBlobStore{listErr: errors.New("list boom")}
	s := store.New(ix, bs, clk, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected list error propagated")
	}
}

func TestStoreReconcileIndexListError(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	ix := mockIndex{listErr: errors.New("index list boom")}
	bs := &mockBlobStore{}
	s := store.New(ix, bs, clk, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected index list error propagated")
	}
}

func TestStoreReconcileDeleteErrorIgnored(t *testing.T) {
	clk := fixedClock{now: time.Now()}
	ix := mockIndex{}
	bs := &mockBlobStore{listIDs: []string{"orphan"}, deleteErr: errors.New("del fail")}
	s := store.New(ix, bs, clk, 10)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("unexpected error despite delete failure: %v", err)
	}
}
