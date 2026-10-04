package store_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/filesystem"
	"github.com/haukened/gone/internal/store/sqlite"
)

// storeFixture holds an integrated store and its backing blob directory.
type storeFixture struct {
	ctx     context.Context
	store   *store.Store
	blobDir string
}

// storeNewFixture creates an integrated store backed by SQLite and filesystem blobs.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - now: fixed clock time.
//   - inlineMax: maximum inline payload size.
//
// Returns: a configured store fixture.
func storeNewFixture(t *testing.T, now time.Time, inlineMax int64) storeFixture {
	t.Helper()
	ix, err := sqlite.New(storeOpenTestDB(t))
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	blobDir := t.TempDir()
	bs, err := filesystem.New(blobDir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	return storeFixture{ctx: context.Background(), store: store.New(ix, bs, storeFixedClock{now: now}, inlineMax), blobDir: blobDir}
}

// storeSave writes a secret through the fixture store.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - f: store fixture.
//   - id: secret id.
//   - meta: protocol metadata.
//   - data: ciphertext bytes to save.
//   - expires: secret expiry time.
//
// Returns: none; failures abort the test.
func storeSave(t *testing.T, f storeFixture, id string, meta app.Meta, data []byte, expires time.Time) {
	t.Helper()
	err := f.store.Save(f.ctx, id, meta, storeTestManageHash, io.NopCloser(storeBytesReader(data)), int64(len(data)), expires)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// storeClaim reads and closes a claimed secret body.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - f: store fixture.
//   - id: secret id.
//   - hash: claim hash.
//   - retry: whether this is a retry claim.
//   - lease: claim lease deadline.
//
// Returns: the claimed metadata and payload bytes.
func storeClaim(t *testing.T, f storeFixture, id, hash string, retry bool, lease time.Time) (app.Claimed, []byte) {
	t.Helper()
	claimed, err := f.store.Claim(f.ctx, id, hash, retry, lease)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	body, err := io.ReadAll(claimed.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := claimed.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return claimed, body
}

// storeAssertClaim verifies a claimed secret matches expected values.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - claimed: claimed secret metadata.
//   - body: claimed payload bytes.
//   - wantData: expected payload bytes.
//   - wantMeta: expected protocol metadata.
//   - wantLease: expected claim lease.
//
// Returns: none; failures abort the test.
func storeAssertClaim(t *testing.T, claimed app.Claimed, body, wantData []byte, wantMeta app.Meta, wantLease time.Time) {
	t.Helper()
	storeAssertPayload(t, body, wantData)
	storeAssertClaimMeta(t, claimed, wantMeta, int64(len(wantData)), wantLease)
}

// storeAssertPayload verifies payload equality.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - got: actual payload bytes.
//   - want: expected payload bytes.
//
// Returns: none; failures abort the test.
func storeAssertPayload(t *testing.T, got, want []byte) {
	t.Helper()
	if string(got) != string(want) {
		t.Fatalf("payload mismatch got=%q want=%q", got, want)
	}
}

// storeAssertClaimMeta verifies claimed metadata fields.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - claimed: actual claimed metadata.
//   - meta: expected protocol metadata.
//   - size: expected payload size.
//   - lease: expected claim lease.
//
// Returns: none; failures abort the test.
func storeAssertClaimMeta(t *testing.T, claimed app.Claimed, meta app.Meta, size int64, lease time.Time) {
	t.Helper()
	if claimed.Size != size {
		t.Fatalf("size mismatch got=%d want=%d", claimed.Size, size)
	}
	if claimed.Meta.Version != meta.Version || claimed.Meta.NonceB64u != meta.NonceB64u {
		t.Fatalf("meta mismatch")
	}
	if claimed.ClaimedUntil.Unix() != lease.Unix() {
		t.Fatalf("lease mismatch got=%v want=%v", claimed.ClaimedUntil, lease)
	}
}

func TestStoreSaveInlineClaimAndAck(t *testing.T) {
	now := time.Now().UTC()
	fixture := storeNewFixture(t, now, 64)
	id := "11111111111111111111111111111111"
	meta := app.Meta{Version: 1, NonceB64u: "nonceA"}
	data := []byte("hello-inline")
	lease := now.Add(time.Minute)
	storeSave(t, fixture, id, meta, data, now.Add(5*time.Minute))
	claimed, body := storeClaim(t, fixture, id, "claim-hash-a", false, lease)
	storeAssertClaim(t, claimed, body, data, meta, lease)
	if _, err := fixture.store.Claim(fixture.ctx, id, "other-hash", false, lease); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for second fresh claim, got %v", err)
	}
	if err := fixture.store.Ack(fixture.ctx, id, "claim-hash-a"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if _, err := fixture.store.Claim(fixture.ctx, id, "claim-hash-a", true, lease); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after ack, got %v", err)
	}
}

func TestStoreSaveExternalClaimAndAckDeletesBlob(t *testing.T) {
	now := time.Now().UTC()
	fixture := storeNewFixture(t, now, 4)
	id := "22222222222222222222222222222222"
	meta := app.Meta{Version: 2, NonceB64u: "nonceB"}
	data := []byte("this-is-external-data")
	lease := now.Add(time.Minute)
	storeSave(t, fixture, id, meta, data, now.Add(10*time.Minute))
	storeAssertBlobExists(t, fixture.blobDir, id)
	claimed, body := storeClaim(t, fixture, id, "claim-hash-b", false, lease)
	storeAssertClaim(t, claimed, body, data, meta, lease)
	storeAssertBlobExists(t, fixture.blobDir, id)
	if err := fixture.store.Ack(fixture.ctx, id, "claim-hash-b"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	storeAssertBlobMissing(t, fixture.blobDir, id)
}

// storeAssertBlobExists verifies that a blob file exists.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//   - id: secret id.
//
// Returns: none; failures abort the test.
func storeAssertBlobExists(t *testing.T, dir, id string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, id+".blob")); err != nil {
		t.Fatalf("expected blob file: %v", err)
	}
}

// storeAssertBlobMissing verifies that a blob file does not exist.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//   - id: secret id.
//
// Returns: none; failures abort the test.
func storeAssertBlobMissing(t *testing.T, dir, id string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, id+".blob")); !os.IsNotExist(err) {
		t.Fatalf("expected blob removed, err=%v", err)
	}
}
