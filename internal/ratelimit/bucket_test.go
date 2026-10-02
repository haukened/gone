package ratelimit

import (
	"testing"
	"time"
)

func TestBucketTake(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name     string
		tokens   float64
		advance  time.Duration
		perSec   float64
		burst    float64
		wantOK   bool
		wantWait time.Duration
		wantLeft float64
	}{
		{name: "full bucket", tokens: 3, perSec: 1, burst: 3, wantOK: true, wantLeft: 2},
		{name: "exactly one", tokens: 1, perSec: 1, burst: 3, wantOK: true, wantLeft: 0},
		{name: "empty denies", tokens: 0, perSec: 1, burst: 3, wantOK: false, wantWait: time.Second},
		{name: "half token wait", tokens: 0.5, perSec: 1, burst: 3, wantOK: false, wantWait: 500 * time.Millisecond, wantLeft: 0.5},
		{name: "slow rate wait", tokens: 0, perSec: 0.5, burst: 3, wantOK: false, wantWait: 2 * time.Second},
		{name: "refill allows", tokens: 0, advance: time.Second, perSec: 1, burst: 3, wantOK: true, wantLeft: 0},
		{name: "refill clamps to burst", tokens: 0, advance: time.Hour, perSec: 1, burst: 3, wantOK: true, wantLeft: 2},
		{name: "backwards clock adds nothing", tokens: 0, advance: -time.Hour, perSec: 1, burst: 3, wantOK: false, wantWait: time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &bucket{tokens: tc.tokens, last: start}
			now := start.Add(tc.advance)
			ok, wait := b.take(now, tc.perSec, tc.burst)
			if ok != tc.wantOK || wait != tc.wantWait {
				t.Fatalf("take() = (%v, %v), want (%v, %v)", ok, wait, tc.wantOK, tc.wantWait)
			}
			if b.tokens != tc.wantLeft {
				t.Fatalf("tokens = %v, want %v", b.tokens, tc.wantLeft)
			}
			if tc.advance < 0 && !b.last.Equal(start) {
				t.Fatalf("last rewound to %v", b.last)
			}
		})
	}
}

func TestBucketFull(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	b := newBucket(start, 2)
	if !b.full(start, 1, 2) {
		t.Fatal("new bucket should be full")
	}
	b.take(start, 1, 2)
	if b.full(start, 1, 2) {
		t.Fatal("drained bucket should not be full")
	}
	if !b.full(start.Add(time.Second), 1, 2) {
		t.Fatal("refilled bucket should be full")
	}
}
