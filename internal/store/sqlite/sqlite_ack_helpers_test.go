package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/internal/store"
)

func sqliteRunAckMatchingCase(t *testing.T, id string, external bool) {
	t.Helper()
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	sqliteInsertAckRow(ctx, t, ix, id, external, now)
	sqliteClaimAckRow(ctx, t, ix, id, now)
	gotExternal, err := ix.Ack(ctx, id, "hash-ack")
	if err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if gotExternal != external {
		t.Fatalf("external got=%v want=%v", gotExternal, external)
	}
	if sqliteRowExists(t, db, id) {
		t.Fatalf("expected ack to delete row")
	}
}

// sqliteInsertAckRow inserts an inline or external row for ack tests.
//
// Parameters:
//   - ctx: request context.
//   - t: test handle used for failure reporting.
//   - ix: index under test.
//   - id: secret id.
//   - external: whether to insert an external row.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteInsertAckRow(ctx context.Context, t *testing.T, ix *Index, id string, external bool, now time.Time) {
	t.Helper()
	if external {
		sqliteInsertExternalSecret(ctx, t, ix, id, 77, now, now.Add(time.Hour))
		return
	}
	sqliteInsertInlineSecret(ctx, t, ix, id, []byte("ack-data"), now, now.Add(time.Hour))
}

// sqliteClaimAckRow claims a row for ack tests.
//
// Parameters:
//   - ctx: request context.
//   - t: test handle used for failure reporting.
//   - ix: index under test.
//   - id: secret id.
//   - now: current time used for claim checks.
//
// Returns: none; failures abort the test.
func sqliteClaimAckRow(ctx context.Context, t *testing.T, ix *Index, id string, now time.Time) {
	t.Helper()
	res, err := ix.Claim(ctx, id, "hash-ack", false, now, now.Add(time.Minute), func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if res.Reader != nil {
		sqliteCloseReader(t, res.Reader)
	}
}

func sqliteSetupBoundaryRows(ctx context.Context, t *testing.T, ix *Index, now, cutoff time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "ttl-boundary", []byte("ttl"), now, cutoff)
	sqliteInsertExternalSecret(ctx, t, ix, "claim-boundary", 42, now, now.Add(time.Hour))
	sqliteInsertInlineSecret(ctx, t, ix, "future", []byte("future"), now, cutoff.Add(time.Second))
}

// sqliteClaimBoundaryRow claims the external boundary row.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - now: current time used for claim checks.
//   - cutoff: lease cutoff under test.
//
// Returns: none; failures abort the test.
func sqliteClaimBoundaryRow(ctx context.Context, t *testing.T, ix *Index, now, cutoff time.Time) {
	t.Helper()
	res, err := ix.Claim(ctx, "claim-boundary", "hash-claim-boundary", false, now, cutoff, func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("Claim boundary row: %v", err)
	}
	if res.Reader != nil {
		sqliteCloseReader(t, res.Reader)
	}
}

// sqliteAssertExpiredRecords verifies boundary expired records.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - recs: records returned by DeleteExpired.
//
// Returns: none; failures abort the test.
func sqliteAssertExpiredRecords(t *testing.T, recs []store.ExpiredRecord) {
	t.Helper()
	if len(recs) != 2 {
		t.Fatalf("expected 2 expired records, got %d (%+v)", len(recs), recs)
	}
	byID := sqliteExpiredRecordsByID(recs)
	sqliteAssertExpiredRecord(t, byID, "ttl-boundary", false, false)
	sqliteAssertExpiredRecord(t, byID, "claim-boundary", true, true)
}

// sqliteExpiredRecordsByID indexes expired records by id.
//
// Parameters:
//   - recs: expired records to index.
//
// Returns: a map keyed by record id.
func sqliteExpiredRecordsByID(recs []store.ExpiredRecord) map[string]store.ExpiredRecord {
	byID := map[string]store.ExpiredRecord{}
	for _, rec := range recs {
		byID[rec.ID] = rec
	}
	return byID
}

// sqliteAssertExpiredRecord verifies one expired record.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - byID: records keyed by id.
//   - id: record id to verify.
//   - claimed: expected claimed flag.
//   - external: expected external flag.
//
// Returns: none; failures abort the test.
func sqliteAssertExpiredRecord(t *testing.T, byID map[string]store.ExpiredRecord, id string, claimed, external bool) {
	t.Helper()
	rec, ok := byID[id]
	if !ok || rec.Claimed != claimed || rec.External != external {
		t.Fatalf("record %q got=%+v ok=%v", id, rec, ok)
	}
}

// sqliteAssertBoundaryRows verifies removed and retained boundary rows.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - db: database under test.
//
// Returns: none; failures abort the test.
func sqliteAssertBoundaryRows(t *testing.T, db *sql.DB) {
	t.Helper()
	if sqliteRowExists(t, db, "ttl-boundary") || sqliteRowExists(t, db, "claim-boundary") {
		t.Fatalf("expected boundary rows removed")
	}
	if !sqliteRowExists(t, db, "future") {
		t.Fatalf("expected future row to remain")
	}
}
