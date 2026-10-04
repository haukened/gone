package sqlrows

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	_ "modernc.org/sqlite"
)

// fakeRows yields n rows and returns the configured errors.
type fakeRows struct {
	n        int
	errIter  error
	errClose error
	closed   bool
}

// Close records the close and returns errClose.
//
// Returns the configured close error.
func (f *fakeRows) Close() error { f.closed = true; return f.errClose }

// Err returns the configured iteration error.
//
// Returns errIter.
func (f *fakeRows) Err() error { return f.errIter }

// Next advances while rows remain.
//
// Returns true while rows remain.
func (f *fakeRows) Next() bool { f.n--; return f.n >= 0 }

// Scan is a no-op.
//
// Parameters:
//   - dest: ignored.
//
// Returns nil.
func (f *fakeRows) Scan(...any) error { return nil }

// TestEach checks iteration counts, error propagation, and that rows are
// always closed.
//
// Parameters:
//   - t: the test handle.
func TestEach(t *testing.T) {
	errRow, errIter, errClose := errors.New("row"), errors.New("iter"), errors.New("close")
	tests := map[string]struct {
		rows      *fakeRows
		rowErr    error
		wantCalls int
		wantErrs  []error
	}{
		"all rows":    {rows: &fakeRows{n: 3}, wantCalls: 3},
		"no rows":     {rows: &fakeRows{}, wantCalls: 0},
		"row error":   {rows: &fakeRows{n: 3}, rowErr: errRow, wantCalls: 1, wantErrs: []error{errRow}},
		"iter error":  {rows: &fakeRows{n: 2, errIter: errIter}, wantCalls: 2, wantErrs: []error{errIter}},
		"close error": {rows: &fakeRows{n: 1, errClose: errClose}, wantCalls: 1, wantErrs: []error{errClose}},
		"row and close": {
			rows: &fakeRows{n: 1, errClose: errClose}, rowErr: errRow,
			wantCalls: 1, wantErrs: []error{errRow, errClose},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			calls := 0
			err := Each(tc.rows, func(Rows) error { calls++; return tc.rowErr })
			if calls != tc.wantCalls || !tc.rows.closed {
				t.Fatalf("calls = %d, closed = %v", calls, tc.rows.closed)
			}
			if len(tc.wantErrs) == 0 && err != nil {
				t.Fatalf("err = %v", err)
			}
			for _, want := range tc.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want %v", err, want)
				}
			}
		})
	}
}

// openTestDB returns an in-memory database seeded with rows 1, 2 and 3.
//
// Parameters:
//   - t: the test handle.
//
// Returns the open database, closed automatically at test cleanup.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE t (v INTEGER); INSERT INTO t VALUES (1), (2), (3)`); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestQuery checks Query against a real database using bind parameters.
//
// Parameters:
//   - t: the test handle.
func TestQuery(t *testing.T) {
	db := openTestDB(t)
	var got []int
	err := Query(context.Background(), db, `SELECT v FROM t WHERE v >= ? ORDER BY v`, func(r Rows) error {
		var v int
		err := r.Scan(&v)
		got = append(got, v)
		return err
	}, 2)
	if err != nil || !slices.Equal(got, []int{2, 3}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

// TestQueryBadStatement checks that a failing statement returns an error.
//
// Parameters:
//   - t: the test handle.
func TestQueryBadStatement(t *testing.T) {
	db := openTestDB(t)
	if err := Query(context.Background(), db, `SELECT nope FROM missing`, func(Rows) error { return nil }); err == nil {
		t.Fatal("want error for bad query")
	}
}
