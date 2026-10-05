package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
	"github.com/haukened/gone/v3/internal/store/filesystem"
)

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
			idx := storeMockIndex{ackExternal: tc.external}
			blobs := &storeMockBlobStore{}
			st := store.New(idx, blobs, storeFixedClock{now: time.Now()}, 4)
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
	st := store.New(storeMockIndex{ackErr: ackErr}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 4)
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
			blobs := &storeMockBlobStore{}
			st := store.New(storeMockIndex{revokeExternal: tc.external}, blobs, storeFixedClock{now: time.Now()}, 4)
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
	blobs := &storeMockBlobStore{}
	st := store.New(storeMockIndex{revokeErr: app.ErrNotFound, revokeExternal: true}, blobs, storeFixedClock{now: time.Now()}, 4)
	if err := st.Revoke(context.Background(), "id", "hash"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if len(blobs.deleteIDs) != 0 {
		t.Fatalf("blob deleted on failed revoke")
	}
}

func TestStoreStatusPassesThrough(t *testing.T) {
	want := app.SecretStatus{CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0)}
	st := store.New(storeMockIndex{status: want}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 4)
	got, err := st.Status(context.Background(), "id", "hash")
	if err != nil || got != want {
		t.Fatalf("Status = %+v, %v", got, err)
	}
}

func TestStoreRevokeExternalRemovesBlobFile(t *testing.T) {
	now := time.Now().UTC()
	fixture := storeNewFixture(t, now, 4)
	id := "77777777777777777777777777777777"
	storeSave(t, fixture, id, app.Meta{Version: 1, NonceB64u: "n"}, []byte("external-payload"), now.Add(time.Hour))
	if _, err := fixture.store.Status(fixture.ctx, id, storeTestManageHash); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := fixture.store.Revoke(fixture.ctx, id, storeTestManageHash); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	storeAssertNoBlobs(t, fixture.blobDir)
	if _, err := fixture.store.Status(fixture.ctx, id, storeTestManageHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Status after revoke = %v", err)
	}
}

// storeAssertNoBlobs verifies the fixture blob directory has no live blobs.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - dir: blob root directory.
//
// Returns: none; failures abort the test.
func storeAssertNoBlobs(t *testing.T, dir string) {
	t.Helper()
	bs, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	ids, err := bs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("blob files remain: %v", ids)
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
