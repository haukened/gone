package sqlite

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

func TestIndexMigrationAddsClaimColumnsAndPreservesRows(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	sqliteCreateLegacyClaimSchema(ctx, t, db)
	sqliteInsertLegacyClaimRow(ctx, t, db, now)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !sqliteHasColumn(t, db, "claim_hash") || !sqliteHasColumn(t, db, "claimed_until") {
		t.Fatalf("expected migration to add claim columns")
	}
	sqliteAssertLegacyClaim(ctx, t, ix, now)
}

func TestIndexClaimInlineBehaviors(t *testing.T) {
	cases := []sqliteClaimInlineCase{
		{name: "fresh claim succeeds", id: "fresh", setup: sqliteSetupFreshClaim, hash: "hash-fresh"},
		{name: "second fresh claim returns not found", id: "second-fresh", setup: sqliteSetupSecondFreshClaim, hash: "hash-other", wantErr: app.ErrNotFound},
		{name: "retry with correct hash succeeds and returns same data", id: "retry-correct", setup: sqliteSetupRetryCorrect, retry: true, hash: "hash-retry"},
		{name: "retry with wrong hash returns not found", id: "retry-wrong", setup: sqliteSetupRetryWrong, retry: true, hash: "hash-intruder", wantErr: app.ErrNotFound, check: sqliteCheckRetryWrong},
		{name: "retry on unclaimed row returns not found", id: "retry-unclaimed", setup: sqliteSetupRetryUnclaimed, retry: true, hash: "hash-missing", wantErr: app.ErrNotFound},
		{name: "claim after ttl deletes row and returns not found", id: "ttl-dead", setup: sqliteSetupTTLDead, hash: "hash-dead", wantErr: app.ErrNotFound, check: sqliteCheckTTLDead},
		{name: "claim after lease lapsed deletes row and returns not found", id: "lease-dead", setup: sqliteSetupLeaseDead, retry: true, hash: "hash-owner", atOffset: time.Minute, wantErr: app.ErrNotFound, check: sqliteCheckLeaseDead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sqliteRunInlineClaimCase(t, tc)
		})
	}
}

func TestIndexClaimExternalOpensBlob(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "external-open"
	now := time.Now().UTC()
	sqliteInsertExternalSecret(ctx, t, ix, id, 1234, now, now.Add(time.Hour))
	rec := &sqliteExternalRecorder{}
	res, err := ix.Claim(ctx, id, "hash-ext", false, now, now.Add(time.Minute), rec.Open)
	if err != nil {
		t.Fatalf("Claim external: %v", err)
	}
	defer sqliteCloseReader(t, res.Reader)
	sqliteAssertExternalClaim(t, res, rec, id)
	sqliteAssertReaderData(t, res.Reader, "blob")
}

func TestIndexClaimExternalOpenFailureLeavesRowUnclaimed(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext-open-failure"
	now := time.Now().UTC()
	sqliteInsertExternalSecret(ctx, t, ix, id, 1234, now, now.Add(time.Hour))
	openErr := errors.New("open failed")
	if _, err := ix.Claim(ctx, id, "hash-failed", false, now, now.Add(time.Minute), func(string) (io.ReadCloser, error) {
		return nil, openErr
	}); !errors.Is(err, openErr) {
		t.Fatalf("expected open error, got %v", err)
	}
	res, err := ix.Claim(ctx, id, "hash-after-failure", false, now.Add(time.Second), now.Add(time.Minute), func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err != nil {
		t.Fatalf("expected fresh claim after failed open to succeed, got %v", err)
	}
	if res.Reader != nil {
		_ = res.Reader.Close()
	}
}

// sqliteCloseReader closes r and reports close errors.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - r: reader to close.
//
// Returns: none; failures abort the test.
func sqliteCloseReader(t *testing.T, r io.Closer) {
	t.Helper()
	if err := r.Close(); err != nil {
		t.Errorf("close reader: %v", err)
	}
}

// sqliteAssertReaderData verifies the next bytes read from r.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - r: source reader.
//   - want: expected string.
//
// Returns: none; failures abort the test.
func sqliteAssertReaderData(t *testing.T, r io.Reader, want string) {
	t.Helper()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != want {
		t.Fatalf("reader data got=%q", got)
	}
}
