package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

func TestIndexAckMatchingHashDeletesRowAndReportsExternal(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		external bool
	}{
		{name: "inline", id: "ack-inline", external: false},
		{name: "external", id: "ack-external", external: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sqliteRunAckMatchingCase(t, tc.id, tc.external)
		})
	}
}

// sqliteRunAckMatchingCase verifies ack deletes one matching claimed row.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - id: secret id to insert and claim.
//   - external: whether the row should be external.
//

func TestIndexAckMismatchedHashReturnsNotFoundAndLeavesRow(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	id := "ack-mismatch"
	sqliteInsertInlineSecret(ctx, t, ix, id, []byte("ack-data"), now, now.Add(time.Hour))
	if _, err := ix.Claim(ctx, id, "hash-owner", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := ix.Ack(ctx, id, "hash-wrong"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if !sqliteRowExists(t, db, id) {
		t.Fatalf("expected row to remain after mismatched ack")
	}
	if _, err := ix.Claim(ctx, id, "hash-owner", true, now.Add(time.Second), now.Add(time.Minute), nil); err != nil {
		t.Fatalf("expected correct retry after mismatched ack to succeed: %v", err)
	}
}

func TestIndexAckMissingReturnsNotFound(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := ix.Ack(context.Background(), "missing", "hash"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIndexAckAllowsLapsedLease(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	id := "ack-lapsed"
	sqliteInsertInlineSecret(ctx, t, ix, id, []byte("ack-data"), now, now.Add(time.Hour))
	if _, err := ix.Claim(ctx, id, "hash-lapsed", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE secrets SET claimed_until=? WHERE id=?`, now.Add(-time.Minute).Unix(), id); err != nil {
		t.Fatalf("force lapsed lease: %v", err)
	}
	if external, err := ix.Ack(ctx, id, "hash-lapsed"); err != nil || external {
		t.Fatalf("Ack after lapsed lease got external=%v err=%v", external, err)
	}
	if sqliteRowExists(t, db, id) {
		t.Fatalf("expected ack to delete row")
	}
}

func TestIndexDeleteExpiredRemovesLapsedClaimsAndBoundaryRows(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(time.Minute)
	sqliteSetupBoundaryRows(ctx, t, ix, now, cutoff)
	sqliteClaimBoundaryRow(ctx, t, ix, now, cutoff)
	recs, err := ix.DeleteExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	sqliteAssertExpiredRecords(t, recs)
	sqliteAssertBoundaryRows(t, db)
}

// sqliteSetupBoundaryRows inserts rows for delete-expired boundary checks.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - now: current time used for timestamps.
//   - cutoff: expiry cutoff under test.
//

func TestIndexListExternalIDs(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	sqliteInsertInlineSecret(ctx, t, ix, "inl", []byte("d"), now, now.Add(5*time.Minute))
	sqliteInsertExternalSecret(ctx, t, ix, "extA", 11, now, now.Add(5*time.Minute))
	sqliteInsertExternalSecret(ctx, t, ix, "extB", 12, now, now.Add(5*time.Minute))
	ids, err := ix.ListExternalIDs(ctx)
	if err != nil {
		t.Fatalf("ListExternalIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 external ids, got %d (%v)", len(ids), ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen["extA"] || !seen["extB"] {
		t.Fatalf("missing expected external IDs: %v", ids)
	}
}
