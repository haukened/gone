package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/store"
)

type sqliteClaimWindow struct {
	now   time.Time
	lease time.Time
}

// sqliteMustClaimInline claims an inline secret and returns the result.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - id: secret id.
//   - hash: claim hash.
//   - retry: retry flag.
//   - window: current time and lease deadline used for claiming.
//
// Returns: the claimed index result.
func sqliteMustClaimInline(ctx context.Context, t *testing.T, ix *Index, id, hash string, retry bool, window sqliteClaimWindow) *store.IndexResult {
	t.Helper()
	res, err := ix.Claim(ctx, id, hash, retry, window.now, window.lease, nil)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	return res
}

// sqliteCheckRetryWrong verifies the original claimant can still retry.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for retry checks.
//
// Returns: none; failures abort the test.
func sqliteCheckRetryWrong(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteMustClaimInline(ctx, t, ix, "retry-wrong", "hash-owner", true, sqliteClaimWindow{now: now.Add(2 * time.Second), lease: now.Add(time.Minute)})
}

// sqliteCheckTTLDead verifies the expired row was deleted.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: unused request context.
//   - ix: unused index.
//   - db: database under test.
//   - now: unused current time.
//
// Returns: none; failures abort the test.
func sqliteCheckTTLDead(_ context.Context, t *testing.T, _ *Index, db *sql.DB, _ time.Time) {
	t.Helper()
	if sqliteRowExists(t, db, "ttl-dead") {
		t.Fatalf("expected expired row deleted")
	}
}

// sqliteCheckLeaseDead verifies the lapsed-lease row was deleted.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: unused request context.
//   - ix: unused index.
//   - db: database under test.
//   - now: unused current time.
//
// Returns: none; failures abort the test.
func sqliteCheckLeaseDead(_ context.Context, t *testing.T, _ *Index, db *sql.DB, _ time.Time) {
	t.Helper()
	if sqliteRowExists(t, db, "lease-dead") {
		t.Fatalf("expected lapsed claim row deleted")
	}
}

// sqliteRunInlineClaimCase runs one inline claim behavior case.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - tc: inline claim case to run.
//
// Returns: none; failures abort the test.
func sqliteRunInlineClaimCase(t *testing.T, tc sqliteClaimInlineCase) {
	t.Helper()
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tc.setup(ctx, t, ix, db, now)
	res, err := ix.Claim(ctx, tc.id, tc.hash, tc.retry, now.Add(tc.atOffset), now.Add(5*time.Minute), nil)
	sqliteAssertInlineClaimResult(t, res, err, tc)
	if tc.check != nil {
		tc.check(ctx, t, ix, db, now)
	}
}

// sqliteAssertInlineClaimResult verifies a claim result against a case.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - res: claim result to verify.
//   - err: claim error to verify.
//   - tc: expected case values.
//
// Returns: none; failures abort the test.
func sqliteAssertInlineClaimResult(t *testing.T, res *store.IndexResult, err error, tc sqliteClaimInlineCase) {
	t.Helper()
	if tc.wantErr != nil {
		sqliteAssertClaimError(t, res, err, tc.wantErr)
		return
	}
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if res.External {
		t.Fatalf("expected inline result")
	}
	if len(res.Inline) == 0 {
		t.Fatalf("expected inline data")
	}
	if res.ClaimHash != tc.hash {
		t.Fatalf("claim hash got=%q want=%q", res.ClaimHash, tc.hash)
	}
}

// sqliteAssertClaimError verifies an expected claim error.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - res: claim result to verify.
//   - err: claim error to verify.
//   - want: expected error.
//
// Returns: none; failures abort the test.
func sqliteAssertClaimError(t *testing.T, res *store.IndexResult, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
	if res != nil {
		t.Fatalf("expected nil result on error")
	}
}

// sqliteExternalRecorder records external blob open calls.
type sqliteExternalRecorder struct {
	openedID string
	opens    int
}

// Open returns a reader and records the opened id.
//
// Parameters:
//   - id: external blob id requested.
//
// Returns: a read closer over test blob bytes and nil error.
func (r *sqliteExternalRecorder) Open(id string) (io.ReadCloser, error) {
	r.openedID = id
	r.opens++
	return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
}

// sqliteAssertExternalClaim verifies an external claim result.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - res: claim result to verify.
//   - rec: recorder that captured open calls.
//   - id: expected opened id.
//
// Returns: none; failures abort the test.
func sqliteAssertExternalClaim(t *testing.T, res *store.IndexResult, rec *sqliteExternalRecorder, id string) {
	t.Helper()
	if !res.External {
		t.Fatalf("expected external=true")
	}
	if rec.openedID != id || rec.opens != 1 {
		t.Fatalf("openExternal id=%q opens=%d", rec.openedID, rec.opens)
	}
	if res.Reader == nil {
		t.Fatalf("expected external reader")
	}
	if res.Size != 1234 {
		t.Fatalf("size mismatch")
	}
}
