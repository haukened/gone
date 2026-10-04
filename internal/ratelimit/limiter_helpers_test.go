package ratelimit

import (
	"net/netip"
	"sync"
	"testing"
	"time"
)

// rateLimitFakeClock is a manually advanced time source.
type rateLimitFakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// now returns the current fake time.
func (c *rateLimitFakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// advance moves the fake time forward by d.
func (c *rateLimitFakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// newRateLimitTestClock returns a fake clock at a stable instant.
func newRateLimitTestClock() *rateLimitFakeClock {
	return &rateLimitFakeClock{t: time.Unix(1_700_000_000, 0)}
}

// rateLimitTestKey parses a network prefix for tests.
func rateLimitTestKey(s string) netip.Prefix {
	return netip.MustParsePrefix(s)
}

// assertRateLimitAllow verifies that a limiter allows one request for key.
func assertRateLimitAllow(t *testing.T, l *Limiter, key netip.Prefix, msg string) {
	t.Helper()
	if ok, _ := l.Allow(key); !ok {
		t.Fatal(msg)
	}
}

// assertRateLimitDeny verifies that a limiter denies one request for key.
func assertRateLimitDeny(t *testing.T, l *Limiter, key netip.Prefix, msg string) {
	t.Helper()
	if ok, _ := l.Allow(key); ok {
		t.Fatal(msg)
	}
}

// rateLimitShardPeers finds one key sharing first's overflow shard and one in another shard.
func rateLimitShardPeers(l *Limiter, first netip.Prefix) (same, other netip.Prefix) {
	want := l.shard(parentOf(first))
	for i := 1; i < 256; i++ {
		k := netip.PrefixFrom(netip.AddrFrom4([4]byte{203, 0, byte(i), 1}), 32)
		if l.shard(parentOf(k)) == want {
			same = k
		} else {
			other = k
		}
	}
	return same, other
}
