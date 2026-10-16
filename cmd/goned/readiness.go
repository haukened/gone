package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

const (
	// readinessTTL is how long a readiness result is reused. /readyz is
	// unauthenticated and not rate limited, so probes within this window share
	// one check instead of each touching the database and the disk.
	readinessTTL = 2 * time.Second
	// readinessTimeout bounds one real check, independent of any caller's
	// request context.
	readinessTimeout = 2 * time.Second
)

// readinessCache memoizes a probe's result for a short window. The mutex is
// held across the probe, so concurrent callers share a single check
// (single-flight) and later callers read the stored result.
type readinessCache struct {
	probe func(context.Context) error
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	checked bool
	at      time.Time
	err     error
}

// check returns the cached result when it is fresher than the TTL, otherwise
// runs the probe once and stores its result (failures included). The probe
// runs on a context detached from ctx, so one caller hanging up cannot poison
// the shared result.
//
// Parameters:
//   - ctx: caller context; only its values are kept.
//
// Returns:
//   - error: the probe's result, possibly from up to ttl ago.
func (c *readinessCache) check(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now := c.now(); c.checked && now.Sub(c.at) < c.ttl {
		return c.err
	}
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), readinessTimeout)
	defer cancel()
	c.err = c.probe(pctx)
	c.checked = true
	c.at = c.now()
	return c.err
}

// readinessCheck reports whether the database answers pings and the blob
// directory is readable. Results are cached for readinessTTL.
//
// Parameters:
//   - db: database to ping.
//   - blobDir: blob directory that must be openable and readable.
//
// Returns:
//   - func(context.Context) error: probe returning nil when ready.
func readinessCheck(db *sql.DB, blobDir string) func(context.Context) error {
	c := &readinessCache{probe: readinessProbe(db, blobDir), ttl: readinessTTL, now: time.Now}
	return c.check
}

// readinessProbe builds the uncached check: ping the database, then verify
// the blob directory.
//
// Parameters:
//   - db: database to ping.
//   - blobDir: blob directory to verify.
//
// Returns:
//   - func(context.Context) error: probe returning nil when ready.
func readinessProbe(db *sql.DB, blobDir string) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		return checkDirReadable(blobDir)
	}
}

// checkDirReadable verifies that dir exists and can be read, by opening it
// and reading at most one entry. It never lists the directory, so its cost
// does not grow with the number of blobs.
//
// Parameters:
//   - dir: directory path.
//
// Returns:
//   - error: nil when dir opens and reads (empty or not).
func checkDirReadable(dir string) error {
	f, err := os.Open(dir) // #nosec G304 -- dir is the operator-configured blob directory, not request input
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
