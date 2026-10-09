package sqlite

import (
	"context"
	"time"
)

// Stats counts what the index holds right now, for metrics. It never reads
// identifiers or ciphertext.
type Stats struct {
	Secrets      int64 // live secrets, including request replies
	SecretBytes  int64 // total ciphertext size of those secrets
	OpenRequests int64 // requests still waiting for a reply
}

// statsQuery counts live rows. A secret is live until its TTL or an
// unacknowledged claim lease lapses (the inverse of expiredWhere); a request
// is open until its reply window closes. It takes three bind parameters, all
// the cutoff unix time.
const statsQuery = `SELECT
	(SELECT COUNT(*) FROM secrets WHERE NOT (` + expiredWhere + `)),
	(SELECT COALESCE(SUM(size), 0) FROM secrets WHERE NOT (` + expiredWhere + `)),
	(SELECT COUNT(*) FROM requests WHERE expires_at > ?)`

// Stats reports live secrets, their total size, and open requests at now.
// Rows the janitor has not swept yet are not counted. It is read-only.
//
// Parameters:
//   - ctx: request context.
//   - now: cutoff for deciding which rows are still live.
//
// Returns the counts, or the query error.
func (i *Index) Stats(ctx context.Context, now time.Time) (Stats, error) {
	t := now.Unix()
	var s Stats
	err := i.db.QueryRowContext(ctx, statsQuery, t, t, t, t, t).Scan(&s.Secrets, &s.SecretBytes, &s.OpenRequests)
	return s, err
}
