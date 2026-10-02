package ratelimit

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced time source.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// now returns the current fake time.
func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// advance moves the fake time forward by d.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Unix(1_700_000_000, 0)}
}

func key(s string) netip.Prefix {
	return netip.MustParsePrefix(s)
}

func TestLimiterBurstThenDeny(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 60, Per: time.Minute}, 3, WithClock(clk.now))
	k := key("192.0.2.1/32")
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow(k); !ok {
			t.Fatalf("request %d denied within burst", i)
		}
	}
	ok, wait := l.Allow(k)
	if ok || wait != time.Second {
		t.Fatalf("Allow after burst = (%v, %v), want (false, 1s)", ok, wait)
	}
	clk.advance(time.Second)
	if ok, _ := l.Allow(k); !ok {
		t.Fatal("request denied after refill")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 1, Per: time.Hour}, 1, WithClock(clk.now))
	a, b := key("192.0.2.1/32"), key("2001:db8::/64")
	if ok, _ := l.Allow(a); !ok {
		t.Fatal("first request for a denied")
	}
	if ok, _ := l.Allow(a); ok {
		t.Fatal("second request for a allowed")
	}
	if ok, _ := l.Allow(b); !ok {
		t.Fatal("first request for b denied")
	}
}

func TestLimiterBurstBelowOne(t *testing.T) {
	tests := []struct {
		name  string
		burst int
	}{
		{name: "zero", burst: 0},
		{name: "negative", burst: -5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newClock()
			l := New(Rate{Count: 1, Per: time.Hour}, tc.burst, WithClock(clk.now))
			k := key("192.0.2.1/32")
			if ok, _ := l.Allow(k); !ok {
				t.Fatal("first request denied")
			}
			if ok, _ := l.Allow(k); ok {
				t.Fatal("second request allowed")
			}
		})
	}
}

// shardPeers finds one untracked key whose parent network lands in the same
// overflow shard as first's, and one whose parent lands in another shard.
// Candidates come from distinct /24s so the parent always differs.
//
// Parameters:
//   - l: the limiter whose seed decides shard placement.
//   - first: the reference key.
//
// Returns:
//   - netip.Prefix: a key sharing first's shard, or the zero Prefix.
//   - netip.Prefix: a key in another shard, or the zero Prefix.
func shardPeers(l *Limiter, first netip.Prefix) (same, other netip.Prefix) {
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

func TestLimiterOverflowShards(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 1, Per: time.Hour}, 1, WithClock(clk.now), WithMaxClients(1))
	if ok, _ := l.Allow(key("192.0.2.1/32")); !ok {
		t.Fatal("tracked client denied")
	}
	first := key("198.51.100.1/32")
	same, other := shardPeers(l, first)
	if !same.IsValid() || !other.IsValid() {
		t.Skip("could not find keys for both shard cases")
	}
	if ok, _ := l.Allow(first); !ok {
		t.Fatal("first overflow client denied")
	}
	if ok, _ := l.Allow(key("198.51.100.2/32")); ok {
		t.Fatal("a neighbor in the same /24 should share the drained bucket")
	}
	if ok, _ := l.Allow(same); ok {
		t.Fatal("client in the same shard should share the drained bucket")
	}
	if ok, _ := l.Allow(other); !ok {
		t.Fatal("client in another shard should not be affected")
	}
	if got := len(l.buckets); got != 1 {
		t.Fatalf("tracked buckets = %d, want 1", got)
	}
}

func TestLimiterPerParentCap(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 1, Per: time.Hour}, 1, WithClock(clk.now))
	l.perParent = 2
	for _, k := range []string{"2001:db8:1:1::/64", "2001:db8:1:2::/64"} {
		if ok, _ := l.Allow(key(k)); !ok {
			t.Fatalf("%s denied within its parent's share", k)
		}
	}
	// The parent /48 is at its cap, so a third /64 is not tracked.
	l.Allow(key("2001:db8:1:3::/64"))
	if _, ok := l.buckets[key("2001:db8:1:3::/64")]; ok {
		t.Fatal("key beyond the parent cap was tracked")
	}
	// Another /48 still gets its own buckets.
	if ok, _ := l.Allow(key("2001:db8:2:1::/64")); !ok {
		t.Fatal("client in another /48 denied")
	}
	if got := len(l.buckets); got != 3 {
		t.Fatalf("tracked buckets = %d, want 3", got)
	}
}

func TestSweepReleasesParentSlots(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 1, Per: time.Second}, 1, WithClock(clk.now))
	l.Allow(key("2001:db8:1:1::/64"))
	l.Allow(key("2001:db8:1:2::/64"))
	parent := key("2001:db8:1::/48")
	if got := l.parents[parent]; got != 2 {
		t.Fatalf("parent count = %d, want 2", got)
	}
	clk.advance(time.Second)
	if removed := l.Sweep(); removed != 2 {
		t.Fatalf("Sweep removed %d, want 2", removed)
	}
	if _, ok := l.parents[parent]; ok {
		t.Fatal("parent entry was not released")
	}
}

func TestParentOf(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "192.0.2.77/32", want: "192.0.2.0/24"},
		{in: "2001:db8:abcd:12::/64", want: "2001:db8:abcd::/48"},
		{in: "::/128", want: "::/48"},
	}
	for _, tc := range tests {
		if got := parentOf(key(tc.in)); got != key(tc.want) {
			t.Errorf("parentOf(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestLimiterDisabledRateAllows(t *testing.T) {
	l := New(Rate{}, 1)
	k := key("192.0.2.1/32")
	for i := 0; i < 5; i++ {
		if ok, wait := l.Allow(k); !ok || wait != 0 {
			t.Fatalf("request %d = (%v, %v), want (true, 0)", i, ok, wait)
		}
	}
	if len(l.buckets) != 0 {
		t.Fatal("disabled limiter should not track clients")
	}
}

func TestWithMaxClientsIgnoresNonPositive(t *testing.T) {
	for _, n := range []int{0, -1} {
		l := New(Rate{Count: 1, Per: time.Second}, 1, WithMaxClients(n))
		if l.max != DefaultMaxClients {
			t.Fatalf("WithMaxClients(%d): max = %d, want %d", n, l.max, DefaultMaxClients)
		}
	}
}

func TestLimiterSweep(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 1, Per: time.Second}, 2, WithClock(clk.now))
	busy, idle := key("192.0.2.1/32"), key("192.0.2.2/32")
	l.Allow(busy)
	l.Allow(busy)
	l.Allow(idle)
	clk.advance(time.Second)
	// idle has refilled (1 + 1 = 2); busy holds 1 token.
	if removed := l.Sweep(); removed != 1 {
		t.Fatalf("Sweep removed %d, want 1", removed)
	}
	if _, ok := l.buckets[busy]; !ok {
		t.Fatal("busy bucket was swept")
	}
	if _, ok := l.buckets[idle]; ok {
		t.Fatal("idle bucket was not swept")
	}
}

func TestSweepLoop(t *testing.T) {
	clk := newClock()
	l := New(Rate{Count: 1, Per: time.Second}, 1, WithClock(clk.now))
	l.Allow(key("192.0.2.1/32"))
	clk.advance(time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		l.sweepLoop(ctx, tick)
		close(done)
	}()
	tick <- time.Time{}
	tick <- time.Time{} // second send ensures the first sweep completed
	cancel()
	<-done

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) != 0 {
		t.Fatalf("buckets = %d after sweep, want 0", len(l.buckets))
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	for _, interval := range []time.Duration{0, time.Hour} {
		l := New(Rate{Count: 1, Per: time.Second}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			l.Run(ctx, interval)
			close(done)
		}()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("Run(%v) did not stop after cancel", interval)
		}
	}
}

func TestLimiterConcurrent(t *testing.T) {
	const workers, perWorker, burst = 8, 50, 100
	l := New(Rate{Count: 1, Per: time.Hour}, burst)
	k := key("192.0.2.1/32")
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if ok, _ := l.Allow(k); ok {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
				if i%10 == 0 {
					l.Sweep()
				}
			}
		}()
	}
	wg.Wait()
	// A refill of at most a few tokens can happen during the test run.
	if allowed < burst || allowed > burst+2 {
		t.Fatalf("allowed = %d, want about %d", allowed, burst)
	}
}

func BenchmarkAllow(b *testing.B) {
	l := New(Rate{Count: 1_000_000, Per: time.Second}, 1000)
	keys := make([]netip.Prefix, 1024)
	for i := range keys {
		keys[i] = netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}), 32)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(keys[i%len(keys)])
	}
}
