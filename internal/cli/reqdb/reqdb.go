// Package reqdb keeps the secret requests made by the gone CLI on this
// device, in a file in the user's config directory. Each row holds what only
// the requester may know: the request's manage token and its private key,
// which never appear in a link (docs/protocol.md section 7.5).
//
// The file is gob-encoded: it is internal state for gone, read and written
// only through this package, and not meant to be opened or edited by hand
// ("gone request list" is the way to see it). It is owner-only (0600) in an
// owner-only directory and is rewritten atomically. A lock file serializes
// changes, so two gone processes never lose each other's requests. Like ssh
// with a private key, a file or directory with loose permissions is refused.
package reqdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FileName is the request file inside the CLI config directory.
const FileName = "requests.gob"

// States a request can be in.
const (
	StateWaiting = "waiting"
	StateReady   = "ready"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
	// minPrefix is the shortest ID prefix Resolve accepts.
	minPrefix = 4
	// formatVersion is the current file layout.
	formatVersion = 1
	// maxFileBytes bounds how much of the file is decoded.
	maxFileBytes = 4 << 20
)

// ErrNoMatch reports that no saved request matches an ID or prefix.
var ErrNoMatch = errors.New("no saved request matches")

// ErrBadPrefix reports an ID argument that is not 4 to 32 hex characters.
var ErrBadPrefix = errors.New("a request ID is 4 to 32 lowercase hex characters")

// prefixRE accepts 4 to 32 lowercase hex characters.
var prefixRE = regexp.MustCompile(`^[0-9a-f]{4,32}$`)

// AmbiguousError reports a prefix that matches more than one request.
type AmbiguousError struct {
	// IDs are the matching request IDs.
	IDs []string
}

// Error describes the ambiguity.
func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("the ID prefix matches %d requests", len(e.IDs))
}

// Row is one saved request.
type Row struct {
	ID          string    // request ID (32 hex)
	Origin      string    // server origin the request was made on
	Label       string    // the requester's own note; never sent anywhere
	ManageToken string    // authorizes status, the claim, and cancel
	PrivateKey  []byte    // raw P-256 scalar
	ReplyLink   string    // the link to send; it cannot open anything
	State       string    // StateWaiting or StateReady, as last seen
	CreatedAt   time.Time // when the request was made
	ExpiresAt   time.Time // reply deadline, or the reply's own expiry once ready
}

// DB is the request file in one directory.
type DB struct{ dir string }

// Open prepares dir (owner-only) for the request file. The file itself is
// created on the first change. On Unix a directory that others can write to,
// or that belongs to another user, is refused (*UnsafeError).
//
// Parameters:
//   - dir: the CLI config directory (…/gone).
//
// Returns the handle or an error.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil { // nosemgrep: incorrect-default-permission
		return nil, err
	}
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	return &DB{dir: dir}, nil
}

// Close releases the handle. Every change is already on disk.
//
// Returns nil.
func (d *DB) Close() error { return nil }

// Put saves or replaces a request.
//
// Parameters:
//   - ctx: context.
//   - r: the request.
//
// Returns an I/O error, if any.
func (d *DB) Put(ctx context.Context, r Row) error {
	return d.update(ctx, func(rows []Row) []Row {
		return append(without(rows, r.ID), r)
	})
}

// SetState records the last seen state and expiry of a request.
//
// Parameters:
//   - ctx: context.
//   - id: request ID.
//   - state: StateWaiting or StateReady.
//   - expires: the expiry the server reported.
//
// Returns an I/O error, if any.
func (d *DB) SetState(ctx context.Context, id, state string, expires time.Time) error {
	return d.update(ctx, func(rows []Row) []Row {
		for i := range rows {
			if rows[i].ID == id {
				rows[i].State, rows[i].ExpiresAt = state, expires
			}
		}
		return rows
	})
}

// Delete forgets a request and its key.
//
// Parameters:
//   - ctx: context.
//   - id: request ID.
//
// Returns an I/O error, if any.
func (d *DB) Delete(ctx context.Context, id string) error {
	return d.update(ctx, func(rows []Row) []Row { return without(rows, id) })
}

// List deletes requests past their expiry, then returns the rest newest
// first.
//
// Parameters:
//   - ctx: context.
//   - now: current time.
//
// Returns the live requests or an error.
func (d *DB) List(ctx context.Context, now time.Time) ([]Row, error) {
	var live []Row
	err := d.update(ctx, func(rows []Row) []Row {
		live = live[:0]
		for _, r := range rows {
			if r.ExpiresAt.After(now) {
				live = append(live, r)
			}
		}
		return live
	})
	if err != nil {
		return nil, err
	}
	out := append([]Row(nil), live...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Resolve finds the one live request whose ID starts with prefix.
//
// Parameters:
//   - ctx: context.
//   - prefix: 4 to 32 lowercase hex characters.
//   - now: current time; expired requests are deleted first.
//
// Returns the request, or ErrBadPrefix, ErrNoMatch, *AmbiguousError, or an
// I/O error.
func (d *DB) Resolve(ctx context.Context, prefix string, now time.Time) (Row, error) {
	prefix = strings.ToLower(prefix)
	if len(prefix) < minPrefix || !prefixRE.MatchString(prefix) {
		return Row{}, ErrBadPrefix
	}
	rows, err := d.List(ctx, now)
	if err != nil {
		return Row{}, err
	}
	var hits []Row
	for _, r := range rows {
		if strings.HasPrefix(r.ID, prefix) {
			hits = append(hits, r)
		}
	}
	return pick(hits)
}

// pick returns the single row of hits.
//
// Parameters:
//   - hits: matching rows.
//
// Returns the row, ErrNoMatch, or *AmbiguousError.
func pick(hits []Row) (Row, error) {
	switch len(hits) {
	case 0:
		return Row{}, ErrNoMatch
	case 1:
		return hits[0], nil
	}
	e := &AmbiguousError{}
	for _, h := range hits {
		e.IDs = append(e.IDs, h.ID)
	}
	return Row{}, e
}

// without returns rows minus the one with id.
//
// Parameters:
//   - rows: rows.
//   - id: ID to drop.
//
// Returns the remaining rows.
func without(rows []Row, id string) []Row {
	out := rows[:0]
	for _, r := range rows {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return out
}
