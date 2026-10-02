package ratelimit

import (
	"context"
	"hash/maphash"
	"net/netip"
	"sync"
	"time"
)

// DefaultMaxClients bounds how many client buckets a Limiter tracks. Once the
// table is full, unseen clients share a small set of overflow buckets.
const DefaultMaxClients = 100_000

// overflowShards is the number of shared buckets used once the table is full.
// Overflow is sharded by a keyed hash of the client's parent network, so a
// flooder drains only the shards its own networks land in.
const overflowShards = 64

// DefaultMaxPerParent caps how many client buckets one parent network (IPv4
// /24, IPv6 /48) may hold. It stops a single cheap IPv6 allocation from
// filling the table with tens of thousands of /64 keys.
const DefaultMaxPerParent = 256

// Parent network sizes used to group client keys.
const (
	parentBitsV4 = 24
	parentBitsV6 = 48
)

// DefaultSweepInterval is how often Run removes buckets that have refilled.
const DefaultSweepInterval = time.Minute

// Limiter applies one Rate to many clients, each keyed by a network prefix.
// It is safe for concurrent use. Construct with New.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[netip.Prefix]*bucket
	parents   map[netip.Prefix]int
	overflow  [overflowShards]*bucket
	seed      maphash.Seed
	perSec    float64
	burst     float64
	max       int
	perParent int
	now       func() time.Time
}

// Option customizes a Limiter.
type Option func(*Limiter)

// WithClock replaces the time source, for tests.
//
// Parameters:
//   - now: function returning the current time.
//
// Returns:
//   - Option: the option.
func WithClock(now func() time.Time) Option {
	return func(l *Limiter) { l.now = now }
}

// WithMaxClients overrides DefaultMaxClients. Values below 1 are ignored.
//
// Parameters:
//   - n: maximum number of tracked client buckets.
//
// Returns:
//   - Option: the option.
func WithMaxClients(n int) Option {
	return func(l *Limiter) {
		if n > 0 {
			l.max = n
		}
	}
}

// New returns a Limiter that allows rate requests per client with bursts of
// up to burst requests. A disabled rate yields a Limiter that allows every
// request. A burst below 1 is raised to 1.
//
// Parameters:
//   - rate: the per-client budget.
//   - burst: bucket capacity.
//   - opts: optional settings.
//
// Returns:
//   - *Limiter: the limiter.
func New(rate Rate, burst int, opts ...Option) *Limiter {
	l := &Limiter{
		buckets:   make(map[netip.Prefix]*bucket),
		parents:   make(map[netip.Prefix]int),
		perSec:    rate.PerSecond(),
		burst:     float64(max(burst, 1)),
		max:       DefaultMaxClients,
		perParent: DefaultMaxPerParent,
		now:       time.Now,
		seed:      maphash.MakeSeed(),
	}
	for _, opt := range opts {
		opt(l)
	}
	start := l.now()
	for i := range l.overflow {
		l.overflow[i] = newBucket(start, l.burst)
	}
	return l
}

// Allow consumes one token for key if one is available.
//
// Parameters:
//   - key: the client key from ClientKey.
//
// Returns:
//   - bool: true when the request may proceed.
//   - time.Duration: when denied, how long until the next token; 0 otherwise.
func (l *Limiter) Allow(key netip.Prefix) (bool, time.Duration) {
	if l.perSec <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	return l.bucketFor(key, now).take(now, l.perSec, l.burst)
}

// bucketFor returns the bucket for key, creating it when there is room and
// falling back to a shared overflow bucket when the table is full or the
// key's parent network already holds its share. The caller must hold l.mu.
//
// Parameters:
//   - key: the client key.
//   - now: the current time.
//
// Returns:
//   - *bucket: the bucket to charge.
func (l *Limiter) bucketFor(key netip.Prefix, now time.Time) *bucket {
	if b, ok := l.buckets[key]; ok {
		return b
	}
	parent := parentOf(key)
	if len(l.buckets) >= l.max || l.parents[parent] >= l.perParent {
		return l.overflow[l.shard(parent)]
	}
	b := newBucket(now, l.burst)
	l.buckets[key] = b
	l.parents[parent]++
	return b
}

// parentOf returns the network grouping key: its /24 for IPv4 or its /48
// for IPv6.
//
// Parameters:
//   - key: the client key.
//
// Returns:
//   - netip.Prefix: the masked parent network.
func parentOf(key netip.Prefix) netip.Prefix {
	bits := parentBitsV6
	if key.Addr().Is4() {
		bits = parentBitsV4
	}
	return netip.PrefixFrom(key.Addr(), bits).Masked()
}

// shard maps a parent network to an overflow bucket index using a
// per-limiter random seed, so clients cannot predict which networks share a
// bucket.
//
// Parameters:
//   - parent: the parent network from parentOf.
//
// Returns:
//   - uint64: an index into l.overflow.
func (l *Limiter) shard(parent netip.Prefix) uint64 {
	b := parent.Addr().As16()
	return maphash.Bytes(l.seed, b[:]) % overflowShards
}

// forget removes key's bucket and releases its parent's slot. The caller
// must hold l.mu.
//
// Parameters:
//   - key: the client key to remove.
func (l *Limiter) forget(key netip.Prefix) {
	delete(l.buckets, key)
	parent := parentOf(key)
	if l.parents[parent] <= 1 {
		delete(l.parents, parent)
		return
	}
	l.parents[parent]--
}

// Sweep removes buckets that have refilled to capacity. A full bucket behaves
// exactly like a new one, so removal never changes a later decision.
//
// Returns:
//   - int: the number of buckets removed.
func (l *Limiter) Sweep() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	removed := 0
	for key, b := range l.buckets {
		if b.full(now, l.perSec, l.burst) {
			l.forget(key)
			removed++
		}
	}
	return removed
}

// Run calls Sweep every interval until ctx is canceled. A non-positive
// interval uses DefaultSweepInterval.
//
// Parameters:
//   - ctx: cancellation signal that stops the loop.
//   - interval: time between sweeps.
func (l *Limiter) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	l.sweepLoop(ctx, t.C)
}

// sweepLoop calls Sweep on each tick until ctx is canceled.
//
// Parameters:
//   - ctx: cancellation signal that stops the loop.
//   - tick: channel delivering sweep signals.
func (l *Limiter) sweepLoop(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			l.Sweep()
		}
	}
}
