package sqlite

import (
	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

// deadRowError signals from inside a transaction that an expired or lapsed
// row was deleted: the deletion must be committed and the row reported as not
// found. external records whether the row's payload was in blob storage, so
// the store can delete the blob too.
type deadRowError struct{ external bool }

// Error describes the signal; it never leaves immediateTx.
//
// Returns a fixed message.
func (deadRowError) Error() string { return "sqlite: dead row deleted" }

// notFound returns the error immediateTx reports once the deletion has
// committed.
//
// Returns store.ErrDeadExternal for an external payload, else app.ErrNotFound.
func (d deadRowError) notFound() error {
	if d.external {
		return store.ErrDeadExternal
	}
	return app.ErrNotFound
}
