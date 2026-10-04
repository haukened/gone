// Package sqlrows runs a query and iterates its result rows, so callers
// only supply the per-row scan. Rows are always closed, and close errors
// are joined into the returned error.
package sqlrows

import (
	"context"
	"database/sql"
	"errors"
)

// Rows is the subset of *sql.Rows that Each needs.
type Rows interface {
	Close() error
	Err() error
	Next() bool
	Scan(dest ...any) error
}

// Querier runs a query; *sql.DB, *sql.Tx and *sql.Conn satisfy it.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

var _ Rows = (*sql.Rows)(nil)

// Query runs query on q and calls row once per result row.
//
// Parameters:
//   - ctx: context for the query.
//   - q: database, transaction, or connection to query.
//   - query: SQL statement.
//   - row: called for each row; it should Scan the current row.
//   - args: bind parameters for query.
//
// Returns the first query, scan, iteration, or close error.
func Query(ctx context.Context, q Querier, query string, row func(Rows) error, args ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return Each(rows, row)
}

// Each calls row for every row in rows, then closes rows.
//
// Parameters:
//   - rows: result set to iterate; always closed on return.
//   - row: called for each row; returning an error stops iteration.
//
// Returns the first error from row or iteration, joined with any close
// error.
func Each(rows Rows, row func(Rows) error) (err error) {
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		if err := row(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
