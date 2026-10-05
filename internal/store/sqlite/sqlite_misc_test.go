package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

func TestIndexInsertDuplicate(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	meta := app.Meta{Version: 1, NonceB64u: "dup"}
	if err := ix.Insert(ctx, store.NewRow{ID: "dup1", Meta: meta, Inline: []byte("a"), Size: 1, ManageHash: sqliteTestManageHash, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := ix.Insert(ctx, store.NewRow{ID: "dup1", Meta: meta, Inline: []byte("b"), Size: 1, ManageHash: sqliteTestManageHash, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err == nil {
		t.Fatalf("expected duplicate insert error")
	}
}

func TestIndexClaimMissing(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := ix.Claim(ctx, "nope", "hash", false, now, now.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIndexClaimBeginImmediateError(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sqliteCloseTestDB(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := ix.Claim(ctx, "any", "hash", false, now, now.Add(time.Minute), nil); err == nil {
		t.Fatalf("expected error from closed DB")
	}
}

func TestIndexDeleteExpiredNone(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	recs, err := ix.DeleteExpired(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired empty: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 recs, got %d", len(recs))
	}
}

func TestIndexDeleteExpiredBeginTxError(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sqliteCloseTestDB(t, db)
	ctx := context.Background()
	if _, err := ix.DeleteExpired(ctx, time.Now()); err == nil {
		t.Fatalf("expected error on closed DB")
	}
}

func TestIndexListExternalIDsClosedDB(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sqliteCloseTestDB(t, db)
	ctx := context.Background()
	if _, err := ix.ListExternalIDs(ctx); err == nil {
		t.Fatalf("expected error querying closed DB")
	}
}

func TestIndexImmediateTx(t *testing.T) {
	sentinel := errors.New("boom")
	cases := []struct {
		name       string
		fnErr      error
		wantErr    error
		wantExists bool
	}{
		{name: "commit on nil", fnErr: nil, wantErr: nil, wantExists: false},
		{name: "commit and not found on dead row", fnErr: errDeadRow, wantErr: app.ErrNotFound, wantExists: false},
		{name: "rollback on not found", fnErr: app.ErrNotFound, wantErr: app.ErrNotFound, wantExists: true},
		{name: "rollback on other error", fnErr: sentinel, wantErr: sentinel, wantExists: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := sqliteOpenTestDB(t)
			ix, err := New(db)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx := context.Background()
			now := time.Now().UTC()
			sqliteInsertInlineSecret(ctx, t, ix, "row", []byte("x"), now, now.Add(time.Hour))
			err = ix.immediateTx(ctx, func(conn *sql.Conn) error {
				if delErr := deleteSecret(ctx, conn, "row"); delErr != nil {
					t.Fatalf("delete: %v", delErr)
				}
				return tc.fnErr
			})
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got := sqliteRowExists(t, db, "row"); got != tc.wantExists {
				t.Fatalf("row exists = %v, want %v", got, tc.wantExists)
			}
		})
	}
}

func TestIndexImmediateTxRollsBackOnPanic(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	sqliteInsertInlineSecret(ctx, t, ix, "row", []byte("x"), now, now.Add(time.Hour))
	sqliteRunImmediateTxPanic(ctx, t, ix)
	if !sqliteRowExists(t, db, "row") {
		t.Fatalf("expected row to survive rolled-back panic")
	}
	if _, err := ix.Claim(ctx, "row", "hash", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("expected lock released after panic, got %v", err)
	}
}

// sqliteRunImmediateTxPanic verifies immediateTx propagates panic to defer paths.
//
// Parameters:
//   - ctx: request context.
//   - t: test handle used for failure reporting.
//   - ix: index under test.
//
// Returns: none; failures abort the test.
func sqliteRunImmediateTxPanic(ctx context.Context, t *testing.T, ix *Index) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic from immediateTx callback")
		}
	}()
	err := ix.immediateTx(ctx, func(conn *sql.Conn) error {
		if err := deleteSecret(ctx, conn, "row"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		panic("boom")
	})
	t.Fatalf("expected panic, got err=%v", err)
}

func TestAttachExternalReader(t *testing.T) {
	openErr := errors.New("open failed")
	cases := []struct {
		name       string
		external   bool
		opener     store.ExternalOpener
		wantErr    bool
		wantReader bool
	}{
		{name: "inline ignores opener", external: false, opener: nil},
		{name: "external without opener", external: true, opener: nil, wantErr: true},
		{name: "external open error", external: true, opener: func(string) (io.ReadCloser, error) { return nil, openErr }, wantErr: true},
		{name: "external opens", external: true, opener: func(string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
		}, wantReader: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := &store.IndexResult{External: tc.external}
			err := attachExternalReader(res, "id", tc.opener)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if (res.Reader != nil) != tc.wantReader {
				t.Fatalf("reader set = %v, want %v", res.Reader != nil, tc.wantReader)
			}
		})
	}
}

func TestHashMatches(t *testing.T) {
	cases := []struct {
		stored, presented string
		want              bool
	}{
		{"", "", false},
		{"", "abc", false},
		{"abc", "abd", false},
		{"abc", "abc", true},
	}
	for _, tc := range cases {
		if got := hashMatches(tc.stored, tc.presented); got != tc.want {
			t.Errorf("hashMatches(%q,%q) = %v, want %v", tc.stored, tc.presented, got, tc.want)
		}
	}
}
