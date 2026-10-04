package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterOverflowShards(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 1, Per: time.Hour}, 1, WithClock(clk.now), WithMaxClients(1))
	assertRateLimitAllow(t, l, rateLimitTestKey("192.0.2.1/32"), "tracked client denied")
	first := rateLimitTestKey("198.51.100.1/32")
	same, other := rateLimitShardPeers(l, first)
	if !same.IsValid() || !other.IsValid() {
		t.Skip("could not find keys for both shard cases")
	}
	assertRateLimitAllow(t, l, first, "first overflow client denied")
	assertRateLimitDeny(t, l, rateLimitTestKey("198.51.100.2/32"), "a neighbor in the same /24 should share the drained bucket")
	assertRateLimitDeny(t, l, same, "client in the same shard should share the drained bucket")
	assertRateLimitAllow(t, l, other, "client in another shard should not be affected")
	if got := len(l.buckets); got != 1 {
		t.Fatalf("tracked buckets = %d, want 1", got)
	}
}

func TestLimiterPerParentCap(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 1, Per: time.Hour}, 1, WithClock(clk.now))
	l.perParent = 2
	for _, k := range []string{"2001:db8:1:1::/64", "2001:db8:1:2::/64"} {
		assertRateLimitAllow(t, l, rateLimitTestKey(k), k+" denied within its parent's share")
	}
	l.Allow(rateLimitTestKey("2001:db8:1:3::/64"))
	if _, ok := l.buckets[rateLimitTestKey("2001:db8:1:3::/64")]; ok {
		t.Fatal("key beyond the parent cap was tracked")
	}
	assertRateLimitAllow(t, l, rateLimitTestKey("2001:db8:2:1::/64"), "client in another /48 denied")
	if got := len(l.buckets); got != 3 {
		t.Fatalf("tracked buckets = %d, want 3", got)
	}
}

func TestParentOf(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "192.0.2.77/32", want: "192.0.2.0/24"},
		{in: "2001:db8:abcd:12::/64", want: "2001:db8:abcd::/48"},
		{in: "::/128", want: "::/48"},
	}
	for _, tc := range tests {
		if got := parentOf(rateLimitTestKey(tc.in)); got != rateLimitTestKey(tc.want) {
			t.Errorf("parentOf(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
