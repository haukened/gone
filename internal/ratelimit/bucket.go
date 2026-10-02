package ratelimit

import (
	"math"
	"time"
)

// bucket is a single token bucket. It is not safe for concurrent use; the
// owning Limiter serializes access.
type bucket struct {
	tokens float64
	last   time.Time
}

// newBucket returns a bucket that starts full.
//
// Parameters:
//   - now: the current time.
//   - burst: the bucket capacity.
//
// Returns:
//   - *bucket: a full bucket last updated at now.
func newBucket(now time.Time, burst float64) *bucket {
	return &bucket{tokens: burst, last: now}
}

// refill adds the tokens earned since the last update, capped at burst. A
// clock that moves backwards adds nothing and does not rewind the bucket.
//
// Parameters:
//   - now: the current time.
//   - perSec: tokens added per second.
//   - burst: the bucket capacity.
func (b *bucket) refill(now time.Time, perSec, burst float64) {
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	b.last = now
	b.tokens = math.Min(burst, b.tokens+elapsed.Seconds()*perSec)
}

// take refills the bucket and consumes one token if one is available.
//
// Parameters:
//   - now: the current time.
//   - perSec: tokens added per second; must be positive.
//   - burst: the bucket capacity.
//
// Returns:
//   - bool: true when a token was consumed.
//   - time.Duration: when denied, how long until a token is available; 0 otherwise.
func (b *bucket) take(now time.Time, perSec, burst float64) (bool, time.Duration) {
	b.refill(now, perSec, burst)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := math.Ceil((1 - b.tokens) / perSec * float64(time.Second))
	return false, time.Duration(wait)
}

// full reports whether the bucket has refilled to capacity, meaning it can be
// discarded without changing any future decision.
//
// Parameters:
//   - now: the current time.
//   - perSec: tokens added per second.
//   - burst: the bucket capacity.
//
// Returns:
//   - bool: true when the bucket holds burst tokens.
func (b *bucket) full(now time.Time, perSec, burst float64) bool {
	b.refill(now, perSec, burst)
	return b.tokens >= burst
}
