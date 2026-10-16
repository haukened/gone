package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
	"github.com/haukened/gone/v3/internal/store/filesystem"
	"github.com/haukened/gone/v3/internal/store/sqlite"
)

// storeAt returns a second store over the fixture's database and blob
// directory whose clock reads at.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - f: store fixture to share storage with.
//   - at: the new store's fixed clock time.
//
// Returns: the store.
func storeAt(t *testing.T, f storeFixture, at time.Time) *store.Store {
	t.Helper()
	ix, err := sqlite.New(f.db)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	bs, err := filesystem.New(f.blobDir)
	if err != nil {
		t.Fatalf("filesystem.New: %v", err)
	}
	return store.New(ix, bs, storeFixedClock{now: at}, 4)
}

// storeDeadOp is an operation run against a dead row.
type storeDeadOp func(s *store.Store, f storeFixture) error

// storeRunDeadOps runs each op in a fresh fixture prepared by kill, which
// leaves the row for id dead and returns the store to run op on, and checks
// that op reports plain app.ErrNotFound and removes the row's blob.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - id: the dead row's id.
//   - kill: fixture setup returning the store to use.
//   - ops: operations by subtest name.
//
// Returns: none; failures abort the test.
func storeRunDeadOps(t *testing.T, id string, kill func(t *testing.T, f storeFixture) *store.Store, ops map[string]storeDeadOp) {
	t.Helper()
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			f := storeNewFixture(t, time.Unix(1700000000, 0).UTC(), 4)
			storeAssertPlainNotFound(t, op(kill(t, f), f))
			storeAssertBlobMissing(t, f.blobDir, id)
		})
	}
}

// storeAssertPlainNotFound checks that err is app.ErrNotFound and not the
// index's ErrDeadExternal signal.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - err: error to check.
//
// Returns: none; failures abort the test.
func storeAssertPlainNotFound(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, app.ErrNotFound) || errors.Is(err, store.ErrDeadExternal) {
		t.Fatalf("err = %v, want plain app.ErrNotFound", err)
	}
}

func TestStoreDeadRowDeletesBlob(t *testing.T) {
	const id = "99999999999999999999999999999999"
	expired := func(t *testing.T, f storeFixture) *store.Store {
		storeSave(t, f, id, app.Meta{Version: 1, NonceB64u: "n"}, []byte("external-data"), time.Unix(1699999000, 0))
		return f.store
	}
	storeRunDeadOps(t, id, expired, map[string]storeDeadOp{
		"claim": func(s *store.Store, f storeFixture) error {
			_, err := s.Claim(f.ctx, id, "claim", false, time.Unix(1700000060, 0))
			return err
		},
		"status": func(s *store.Store, f storeFixture) error {
			_, err := s.Status(f.ctx, id, storeTestManageHash)
			return err
		},
		"revoke": func(s *store.Store, f storeFixture) error { return s.Revoke(f.ctx, id, storeTestManageHash) },
	})
}

func TestStoreDeadReplyDeletesBlob(t *testing.T) {
	answeredLongAgo := func(t *testing.T, f storeFixture) *store.Store {
		now := time.Unix(1700000000, 0).UTC()
		reqCreate(t, f, now)
		if _, err := reqFill(f, []byte("larger than inline")); err != nil {
			t.Fatalf("Fill: %v", err)
		}
		return storeAt(t, f, now.Add(31*time.Minute))
	}
	storeRunDeadOps(t, reqID, answeredLongAgo, map[string]storeDeadOp{
		"claim reply": func(s *store.Store, f storeFixture) error {
			_, err := s.ClaimReply(f.ctx, reqID, reqManageHash, "claim", false, time.Unix(1700005000, 0))
			return err
		},
		"cancel": func(s *store.Store, f storeFixture) error { return s.CancelRequest(f.ctx, reqID, reqManageHash) },
	})
}

func TestStoreSweepDeadWithoutBlobStorage(t *testing.T) {
	s := store.New(storeMockIndex{statusErr: store.ErrDeadExternal}, nil, storeFixedClock{now: time.Now()}, 4)
	_, err := s.Status(t.Context(), "id", "manage")
	storeAssertPlainNotFound(t, err)
}
