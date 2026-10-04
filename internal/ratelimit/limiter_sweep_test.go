package ratelimit

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func TestSweepReleasesParentSlots(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 1, Per: time.Second}, 1, WithClock(clk.now))
	l.Allow(rateLimitTestKey("2001:db8:1:1::/64"))
	l.Allow(rateLimitTestKey("2001:db8:1:2::/64"))
	parent := rateLimitTestKey("2001:db8:1::/48")
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

func TestLimiterSweep(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 1, Per: time.Second}, 2, WithClock(clk.now))
	busy := rateLimitTestKey("192.0.2.1/32")
	idle := rateLimitTestKey("192.0.2.2/32")
	l.Allow(busy)
	l.Allow(busy)
	l.Allow(idle)
	clk.advance(time.Second)
	if removed := l.Sweep(); removed != 1 {
		t.Fatalf("Sweep removed %d, want 1", removed)
	}
	assertRateLimitSweepState(t, l, busy, idle)
}

// assertRateLimitSweepState verifies busy remains tracked and idle was swept.
func assertRateLimitSweepState(t *testing.T, l *Limiter, busy, idle netip.Prefix) {
	t.Helper()
	if _, ok := l.buckets[busy]; !ok {
		t.Fatal("busy bucket was swept")
	}
	if _, ok := l.buckets[idle]; ok {
		t.Fatal("idle bucket was not swept")
	}
}

func TestSweepLoop(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 1, Per: time.Second}, 1, WithClock(clk.now))
	l.Allow(rateLimitTestKey("192.0.2.1/32"))
	clk.advance(time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		l.sweepLoop(ctx, tick)
		close(done)
	}()
	tick <- time.Time{}
	tick <- time.Time{}
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
		t.Run(interval.String(), func(t *testing.T) {
			assertRunStopsOnCancel(t, interval)
		})
	}
}

// assertRunStopsOnCancel verifies Run exits after context cancellation.
func assertRunStopsOnCancel(t *testing.T, interval time.Duration) {
	t.Helper()
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
