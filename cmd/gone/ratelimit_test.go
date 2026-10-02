package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/httpx"
	"github.com/haukened/gone/internal/ratelimit"
)

func TestStartLimiter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tests := []struct {
		name    string
		rate    ratelimit.Rate
		wantNil bool
	}{
		{name: "disabled", rate: ratelimit.Rate{}, wantNil: true},
		{name: "enabled", rate: ratelimit.Rate{Count: 1, Per: time.Minute}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := startLimiter(ctx, "test", tc.rate, 1)
			// A typed nil inside the interface would defeat the handler's nil check.
			if (l == nil) != tc.wantNil {
				t.Fatalf("startLimiter() = %v, wantNil %v", l, tc.wantNil)
			}
			if l == nil {
				return
			}
			key := netip.MustParsePrefix("192.0.2.1/32")
			if ok, _ := l.Allow(key); !ok {
				t.Fatal("first request denied")
			}
			if ok, _ := l.Allow(key); ok {
				t.Fatal("second request allowed with burst 1")
			}
		})
	}
}

// fakeMetrics counts increments.
type fakeMetrics struct{ n int64 }

// Inc implements httpx.Metrics.
func (f *fakeMetrics) Inc(_ string, d int64) { f.n += d }

func TestApplyRateLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cfg := &config.Config{
		CreateRate:      ratelimit.Rate{Count: 1, Per: time.Hour},
		RateBurst:       1,
		TrustedPrefixes: trusted,
	}
	m := &fakeMetrics{}
	h := httpx.New(nil, 1024, nil)
	applyRateLimits(ctx, h, cfg, m)
	if h.CreateLimiter == nil || h.ReadLimiter != nil {
		t.Fatalf("create=%v read=%v, want create only", h.CreateLimiter, h.ReadLimiter)
	}
	if len(h.TrustedProxies) != 1 || h.TrustedProxies[0] != trusted[0] || h.Metrics != m {
		t.Fatal("trusted proxies or metrics not applied")
	}

	router := h.Router()
	codes := make([]int, 2)
	for i := range codes {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/secret", nil))
		codes[i] = rr.Code
	}
	if codes[0] == http.StatusTooManyRequests || codes[1] != http.StatusTooManyRequests {
		t.Fatalf("status codes = %v, want second request limited", codes)
	}
	if m.n != 1 {
		t.Fatalf("metric increments = %d, want 1", m.n)
	}
}

var _ httpx.Metrics = (*fakeMetrics)(nil)
