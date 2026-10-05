package store_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
	"github.com/haukened/gone/v3/internal/store/sqlite"
)

func TestStoreClaimExternalOpenFailureIsRetryable(t *testing.T) {
	now := time.Now().UTC()
	blobs := &storeFlakyOpenBlobStore{failOpen: true}
	st := store.New(storeOpenIndex(t), blobs, storeFixedClock{now: now}, 4)
	id := "22222222222222222222222222222223"
	data := []byte("this-is-external-data")
	lease := now.Add(time.Minute)
	if err := st.Save(context.Background(), id, app.Meta{Version: 2, NonceB64u: "nonceB"}, storeTestManageHash, io.NopCloser(storeBytesReader(data)), int64(len(data)), now.Add(10*time.Minute)); err != nil {
		t.Fatalf("Save external: %v", err)
	}
	if _, err := st.Claim(context.Background(), id, "claim-hash-c", false, lease); !errors.Is(err, errStoreBlobOpen) {
		t.Fatalf("expected blob open error, got %v", err)
	}
	blobs.failOpen = false
	claimed, body := storeClaim(t, storeFixture{ctx: context.Background(), store: st}, id, "claim-hash-c", false, lease)
	storeAssertClaimMeta(t, claimed, app.Meta{Version: 2, NonceB64u: "nonceB"}, int64(len(data)), lease)
	storeAssertPayload(t, body, data)
	if blobs.deleted {
		t.Fatalf("did not expect blob deleted before ack")
	}
	if err := st.Ack(context.Background(), id, "claim-hash-c"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if !blobs.deleted {
		t.Fatalf("expected blob deleted after ack")
	}
}

// storeOpenIndex returns a SQLite index for store tests.
//
// Parameters:
//   - t: test handle used for failure reporting.
//
// Returns: a ready SQLite-backed store index.
func storeOpenIndex(t *testing.T) store.Index {
	t.Helper()
	ix, err := sqliteNewStoreIndex(t)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	return ix
}

// sqliteNewStoreIndex constructs a concrete index without exporting sqlite imports to tests.
//
// Parameters:
//   - t: test handle used to create a temporary DB.
//
// Returns: the store index and any constructor error.
func sqliteNewStoreIndex(t *testing.T) (store.Index, error) {
	t.Helper()
	return sqlite.New(storeOpenTestDB(t))
}

func TestStoreClaimExternalMissingReader(t *testing.T) {
	st := store.New(storeMockIndex{claimResult: &store.IndexResult{External: true, Size: 7}}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 4)
	_, err := st.Claim(context.Background(), "id", "hash", false, time.Now().Add(time.Minute))
	if err == nil || err.Error() != "external reader missing" {
		t.Fatalf("expected missing external reader error, got %v", err)
	}
}

func TestStoreClaimExpired(t *testing.T) {
	now := time.Now().UTC()
	fixture := storeNewFixture(t, now, 64)
	id := "33333333333333333333333333333333"
	storeSave(t, fixture, id, app.Meta{Version: 1, NonceB64u: "nC"}, []byte("x"), now.Add(-time.Minute))
	if _, err := fixture.store.Claim(fixture.ctx, id, "claim-hash-d", false, now.Add(time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired claim, got %v", err)
	}
}
