package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

type sqliteClaimInlineCase struct {
	name     string
	id       string
	setup    func(ctx context.Context, t *testing.T, ix *Index, db *sql.DB, now time.Time)
	retry    bool
	hash     string
	atOffset time.Duration
	wantErr  error
	check    func(ctx context.Context, t *testing.T, ix *Index, db *sql.DB, now time.Time)
}

// sqliteCreateLegacyClaimSchema creates the pre-claim migration schema.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - db: database under test.
//
// Returns: none; failures abort the test.
func sqliteCreateLegacyClaimSchema(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	oldSchema := `CREATE TABLE secrets (
id TEXT PRIMARY KEY,
version INTEGER NOT NULL,
nonce_b64u TEXT NOT NULL,
inline BLOB,
external INTEGER NOT NULL DEFAULT 0,
size INTEGER NOT NULL,
created_at INTEGER NOT NULL,
expires_at INTEGER NOT NULL
);`
	if _, err := db.ExecContext(ctx, oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
}

// sqliteInsertLegacyClaimRow inserts a row into the pre-claim schema.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - db: database under test.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteInsertLegacyClaimRow(ctx context.Context, t *testing.T, db *sql.DB, now time.Time) {
	t.Helper()
	query := `INSERT INTO secrets (id, version, nonce_b64u, inline, external, size, created_at, expires_at) VALUES (?,?,?,?,?,?,?,?)`
	_, err := db.ExecContext(ctx, query, "legacy", 1, "nonce-legacy", []byte("legacy-data"), 0, len("legacy-data"), now.Unix(), now.Add(time.Hour).Unix())
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
}

// sqliteAssertLegacyClaim verifies legacy data survives migration and claim.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ix: index under test.
//   - ctx: request context.
//   - now: current time used for claiming.
//
// Returns: none; failures abort the test.
func sqliteAssertLegacyClaim(ctx context.Context, t *testing.T, ix *Index, now time.Time) {
	t.Helper()
	res, err := ix.Claim(ctx, "legacy", "hash-legacy", false, now, now.Add(time.Minute), nil)
	if err != nil {
		t.Fatalf("claim legacy row: %v", err)
	}
	if string(res.Inline) != "legacy-data" {
		t.Fatalf("legacy data got=%q", res.Inline)
	}
}

// sqliteSetupFreshClaim prepares an unclaimed inline row.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupFreshClaim(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "fresh", []byte("fresh-data"), now, now.Add(time.Hour))
}

// sqliteSetupSecondFreshClaim prepares a row already freshly claimed.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupSecondFreshClaim(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "second-fresh", []byte("claimed-data"), now, now.Add(time.Hour))
	sqliteMustClaimInline(ctx, t, ix, "second-fresh", "hash-owner", false, sqliteClaimWindow{now: now, lease: now.Add(time.Minute)})
}

// sqliteSetupRetryCorrect prepares a claimed row and validates its first payload.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupRetryCorrect(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "retry-correct", []byte("retry-data"), now, now.Add(time.Hour))
	first := sqliteMustClaimInline(ctx, t, ix, "retry-correct", "hash-retry", false, sqliteClaimWindow{now: now, lease: now.Add(time.Minute)})
	if string(first.Inline) != "retry-data" {
		t.Fatalf("initial data got=%q", first.Inline)
	}
}

// sqliteSetupRetryWrong prepares a row claimed by a different hash.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupRetryWrong(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "retry-wrong", []byte("retry-data"), now, now.Add(time.Hour))
	sqliteMustClaimInline(ctx, t, ix, "retry-wrong", "hash-owner", false, sqliteClaimWindow{now: now, lease: now.Add(time.Minute)})
}

// sqliteSetupRetryUnclaimed prepares an unclaimed row for a retry attempt.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupRetryUnclaimed(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "retry-unclaimed", []byte("unclaimed-data"), now, now.Add(time.Hour))
}

// sqliteSetupTTLDead prepares an expired row.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupTTLDead(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "ttl-dead", []byte("dead-data"), now.Add(-time.Hour), now)
}

// sqliteSetupLeaseDead prepares a claimed row with a soon-lapsing lease.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - ctx: request context.
//   - ix: index under test.
//   - db: unused database handle.
//   - now: current time used for timestamps.
//
// Returns: none; failures abort the test.
func sqliteSetupLeaseDead(ctx context.Context, t *testing.T, ix *Index, _ *sql.DB, now time.Time) {
	t.Helper()
	sqliteInsertInlineSecret(ctx, t, ix, "lease-dead", []byte("lease-data"), now, now.Add(time.Hour))
	sqliteMustClaimInline(ctx, t, ix, "lease-dead", "hash-owner", false, sqliteClaimWindow{now: now, lease: now.Add(time.Minute)})
}
