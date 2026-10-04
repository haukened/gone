package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/haukened/gone/internal/app"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// writeWait bounds how long a write waits for its turn before giving up with
// app.ErrBusy. It matches the busy_timeout pragma in config.SQLiteDSNFor.
var writeWait = 5 * time.Second

// acquireWrite waits for this Index's write turn. Writers are served in
// arrival order: blocked channel sends queue first in, first out. Left to
// SQLite's busy handler instead, waiting writers poll with sleeps of up to
// 100ms and a late arrival can take the lock ahead of them, so under load
// some writes stall for seconds while others go straight through.
//
// Parameters:
//   - ctx: request context; waiting stops when it is done.
//
// Returns:
//   - func(): releases the turn; call it once the write has committed or
//     rolled back.
//   - error: app.ErrBusy after writeWait, or the context's error.
func (i *Index) acquireWrite(ctx context.Context) (func(), error) {
	if i.writeGate == nil {
		return func() {}, nil
	}
	timer := time.NewTimer(writeWait)
	defer timer.Stop()
	select {
	case i.writeGate <- struct{}{}:
		return func() { <-i.writeGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("%w: waited %s for the write lock", app.ErrBusy, writeWait)
	}
}

// busyError marks SQLite "database is locked" errors as app.ErrBusy. Writes
// outside this Index (such as metrics flushes) share SQLite's lock, so a write
// can still time out inside SQLite after its turn here.
//
// Parameters:
//   - err: an error from the driver, or nil.
//
// Returns:
//   - error: err wrapped with app.ErrBusy when SQLite reported SQLITE_BUSY;
//     otherwise err unchanged.
func busyError(err error) error {
	var se *sqlite.Error
	if errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_BUSY {
		return fmt.Errorf("%w: %w", app.ErrBusy, err)
	}
	return err
}
