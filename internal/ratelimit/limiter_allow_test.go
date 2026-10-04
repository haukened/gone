package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterBurstThenDeny(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 60, Per: time.Minute}, 3, WithClock(clk.now))
	k := rateLimitTestKey("192.0.2.1/32")
	for i := 0; i < 3; i++ {
		assertRateLimitAllow(t, l, k, "request denied within burst")
	}
	ok, wait := l.Allow(k)
	if ok || wait != time.Second {
		t.Fatalf("Allow after burst = (%v, %v), want (false, 1s)", ok, wait)
	}
	clk.advance(time.Second)
	assertRateLimitAllow(t, l, k, "request denied after refill")
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	clk := newRateLimitTestClock()
	l := New(Rate{Count: 1, Per: time.Hour}, 1, WithClock(clk.now))
	a := rateLimitTestKey("192.0.2.1/32")
	b := rateLimitTestKey("2001:db8::/64")
	assertRateLimitAllow(t, l, a, "first request for a denied")
	assertRateLimitDeny(t, l, a, "second request for a allowed")
	assertRateLimitAllow(t, l, b, "first request for b denied")
}

func TestLimiterBurstBelowOne(t *testing.T) {
	for _, tc := range []struct {
		name  string
		burst int
	}{{name: "zero", burst: 0}, {name: "negative", burst: -5}} {
		t.Run(tc.name, func(t *testing.T) {
			clk := newRateLimitTestClock()
			l := New(Rate{Count: 1, Per: time.Hour}, tc.burst, WithClock(clk.now))
			k := rateLimitTestKey("192.0.2.1/32")
			assertRateLimitAllow(t, l, k, "first request denied")
			assertRateLimitDeny(t, l, k, "second request allowed")
		})
	}
}

func TestLimiterDisabledRateAllows(t *testing.T) {
	l := New(Rate{}, 1)
	k := rateLimitTestKey("192.0.2.1/32")
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
