package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
)

// sqliteNewManageIndex opens a fresh database and Index for manage tests.
func sqliteNewManageIndex(t *testing.T) (*Index, *sql.DB) {
	t.Helper()
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ix, db
}

func TestIndexStatus(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	expires := now.Add(time.Hour)
	tests := []struct {
		name    string
		setup   func(t *testing.T, ix *Index, db *sql.DB)
		hash    string
		at      time.Time
		wantErr error
		wantRow bool // row still present afterwards
	}{
		{name: "pending", hash: sqliteTestManageHash, at: now, wantRow: true},
		{name: "wrong hash", hash: "deadbeef", at: now, wantErr: app.ErrNotFound, wantRow: true},
		{name: "empty hash", hash: "", at: now, wantErr: app.ErrNotFound, wantRow: true},
		{name: "expired deletes row", hash: sqliteTestManageHash, at: expires, wantErr: app.ErrNotFound},
		{name: "expired wrong hash still deletes", hash: "deadbeef", at: expires, wantErr: app.ErrNotFound},
		{
			name: "active lease is pending",
			setup: func(t *testing.T, ix *Index, _ *sql.DB) {
				if _, err := ix.Claim(context.Background(), "s", "claim", false, now, now.Add(time.Minute), nil); err != nil {
					t.Fatalf("claim: %v", err)
				}
			},
			hash: sqliteTestManageHash, at: now.Add(30 * time.Second), wantRow: true,
		},
		{
			name: "lapsed lease deletes row",
			setup: func(t *testing.T, ix *Index, _ *sql.DB) {
				if _, err := ix.Claim(context.Background(), "s", "claim", false, now, now.Add(time.Minute), nil); err != nil {
					t.Fatalf("claim: %v", err)
				}
			},
			hash: sqliteTestManageHash, at: now.Add(2 * time.Minute), wantErr: app.ErrNotFound,
		},
		{
			name: "legacy null hash",
			setup: func(t *testing.T, _ *Index, db *sql.DB) {
				if _, err := db.Exec(`UPDATE secrets SET manage_hash=NULL WHERE id='s'`); err != nil {
					t.Fatalf("null hash: %v", err)
				}
			},
			hash: sqliteTestManageHash, at: now, wantErr: app.ErrNotFound, wantRow: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ix, db := sqliteNewManageIndex(t)
			ctx := context.Background()
			sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("data"), now, expires)
			if tc.setup != nil {
				tc.setup(t, ix, db)
			}
			st, err := ix.Status(ctx, "s", tc.hash, tc.at)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Status err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && (!st.CreatedAt.Equal(now) || !st.ExpiresAt.Equal(expires)) {
				t.Fatalf("Status = %+v", st)
			}
			if got := sqliteRowExists(t, db, "s"); got != tc.wantRow {
				t.Fatalf("row exists = %v, want %v", got, tc.wantRow)
			}
		})
	}
}

func TestIndexStatusUnknownID(t *testing.T) {
	ix, _ := sqliteNewManageIndex(t)
	if _, err := ix.Status(context.Background(), "missing", sqliteTestManageHash, time.Now()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Status err = %v, want ErrNotFound", err)
	}
}

func TestIndexRevoke(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	tests := []sqliteRevokeCase{
		{name: "inline pending", hash: sqliteTestManageHash, at: now},
		{name: "external pending reports external", external: true, hash: sqliteTestManageHash, at: now, wantExternal: true},
		{name: "wins over active lease", claimFirst: true, hash: sqliteTestManageHash, at: now.Add(10 * time.Second)},
		{name: "wrong hash", hash: "deadbeef", at: now, wantErr: app.ErrNotFound},
		{name: "expired", hash: sqliteTestManageHash, at: now.Add(time.Hour), wantErr: app.ErrNotFound},
		{name: "lapsed lease", claimFirst: true, hash: sqliteTestManageHash, at: now.Add(2 * time.Minute), wantErr: app.ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sqliteRunRevokeCase(t, now, tc)
		})
	}
}

type sqliteRevokeCase struct {
	name         string
	external     bool
	claimFirst   bool
	hash         string
	at           time.Time
	wantErr      error
	wantExternal bool
}

// sqliteRunRevokeCase verifies one revoke behavior.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - now: base time for setup.
//   - tc: revoke case values.
//
// Returns: none; failures abort the test.
func sqliteRunRevokeCase(t *testing.T, now time.Time, tc sqliteRevokeCase) {
	t.Helper()
	ix, db := sqliteNewManageIndex(t)
	ctx := context.Background()
	sqliteInsertRevokeRow(ctx, t, ix, tc.external, now)
	if tc.claimFirst {
		sqliteMustClaimInline(ctx, t, ix, "s", "claim", false, sqliteClaimWindow{now: now, lease: now.Add(time.Minute)})
	}
	ext, err := ix.Revoke(ctx, "s", tc.hash, tc.at)
	if !errors.Is(err, tc.wantErr) {
		t.Fatalf("Revoke err = %v, want %v", err, tc.wantErr)
	}
	if ext != tc.wantExternal {
		t.Fatalf("external = %v, want %v", ext, tc.wantExternal)
	}
	wantRow := errors.Is(tc.wantErr, app.ErrNotFound) && tc.hash != sqliteTestManageHash
	if got := sqliteRowExists(t, db, "s"); got != wantRow {
		t.Fatalf("row exists = %v, want %v", got, wantRow)
	}
}

// sqliteInsertRevokeRow inserts the row used by a revoke case.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - external: whether to insert an external row.
//   - now: base time for row timestamps.
//
// Returns: none; failures abort the test.
func sqliteInsertRevokeRow(ctx context.Context, t *testing.T, ix *Index, external bool, now time.Time) {
	t.Helper()
	if external {
		sqliteInsertExternalSecret(ctx, t, ix, "s", 10, now, now.Add(time.Hour))
		return
	}
	sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("data"), now, now.Add(time.Hour))
}

func TestIndexRevokeThenClaimOrAckNotFound(t *testing.T) {
	ix, _ := sqliteNewManageIndex(t)
	ctx := context.Background()
	now := time.Now().UTC()
	sqliteInsertInlineSecret(ctx, t, ix, "a", []byte("data"), now, now.Add(time.Hour))
	sqliteInsertInlineSecret(ctx, t, ix, "b", []byte("data"), now, now.Add(time.Hour))

	if _, err := ix.Revoke(ctx, "a", sqliteTestManageHash, now); err != nil {
		t.Fatalf("revoke a: %v", err)
	}
	if _, err := ix.Claim(ctx, "a", "claim", false, now, now.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("claim after revoke err = %v", err)
	}

	if _, err := ix.Claim(ctx, "b", "claim", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("claim b: %v", err)
	}
	if _, err := ix.Revoke(ctx, "b", sqliteTestManageHash, now); err != nil {
		t.Fatalf("revoke b during lease: %v", err)
	}
	if _, err := ix.Ack(ctx, "b", "claim"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ack after revoke err = %v", err)
	}
	if _, err := ix.Revoke(ctx, "b", sqliteTestManageHash, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("second revoke err = %v", err)
	}
}

func TestIndexRevokeAfterAckNotFound(t *testing.T) {
	ix, _ := sqliteNewManageIndex(t)
	ctx := context.Background()
	now := time.Now().UTC()
	sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("data"), now, now.Add(time.Hour))
	if _, err := ix.Claim(ctx, "s", "claim", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := ix.Ack(ctx, "s", "claim"); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if _, err := ix.Revoke(ctx, "s", sqliteTestManageHash, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("revoke after ack err = %v", err)
	}
}

func TestIndexRevokeConcurrentWithClaim(t *testing.T) {
	for i := 0; i < 20; i++ {
		ix, db := sqliteNewManageIndex(t)
		ctx := context.Background()
		now := time.Now().UTC()
		sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("data"), now, now.Add(time.Hour))
		start := make(chan struct{})
		var wg sync.WaitGroup
		var revokeErr, claimErr error
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, revokeErr = ix.Revoke(ctx, "s", sqliteTestManageHash, now) }()
		go func() {
			defer wg.Done()
			<-start
			_, claimErr = ix.Claim(ctx, "s", "claim", false, now, now.Add(time.Minute), nil)
		}()
		close(start)
		wg.Wait()
		if revokeErr != nil {
			t.Fatalf("revoke must always win, got %v", revokeErr)
		}
		if claimErr != nil && !errors.Is(claimErr, app.ErrNotFound) {
			t.Fatalf("claim err = %v", claimErr)
		}
		if sqliteRowExists(t, db, "s") {
			t.Fatalf("row survived revoke")
		}
	}
}
