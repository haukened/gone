package store

import (
	"errors"
	"fmt"

	"github.com/haukened/gone/v3/internal/app"
)

// ErrDeadExternal is returned by Index methods that lazily delete a row past
// its TTL or claim lease when that row's payload was in blob storage. It wraps
// app.ErrNotFound; the Store deletes the blob and reports app.ErrNotFound.
var ErrDeadExternal = fmt.Errorf("%w: dead row with external payload deleted", app.ErrNotFound)

// sweepDead deletes the blob of a dead row that the index has just deleted.
// The deletion is best-effort, like the Store's other blob deletions;
// Reconcile removes any orphan left by a failure here.
//
// Parameters:
//   - id: secret or request identifier.
//   - err: error returned by the index call.
//
// Returns app.ErrNotFound for ErrDeadExternal, otherwise err unchanged.
func (s *Store) sweepDead(id string, err error) error {
	if !errors.Is(err, ErrDeadExternal) {
		return err
	}
	if s.blobs != nil {
		_ = s.blobs.Delete(id)
	}
	return app.ErrNotFound
}
