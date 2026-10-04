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

// TestStartLimiter verifies disabled and enabled limiter construction.
//
// Parameters:
//   - t: the test handle.
func TestStartLimiter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, tc := range limiterCases {
		t.Run(tc.name, func(t *testing.T) {
			l := startLimiter(ctx, "test", tc.rate, 1)
			checkStartedLimiter(t, l, tc.wantNil)
		})
	}
}

var limiterCases = []struct {
	name    string
	rate    ratelimit.Rate
	wantNil bool
}{
	{name: "disabled", rate: ratelimit.Rate{}, wantNil: true},
	{name: "enabled", rate: ratelimit.Rate{Count: 1, Per: time.Minute}},
}

// checkStartedLimiter verifies a limiter's nilness and burst behavior.
//
// Parameters:
//   - t: the test handle.
//   - l: the limiter returned by startLimiter.
//   - wantNil: whether the limiter should be nil.
func checkStartedLimiter(t *testing.T, l interface {
	Allow(netip.Prefix) (bool, time.Duration)
}, wantNil bool) {
	t.Helper()
	if (l == nil) != wantNil {
		t.Fatalf("startLimiter() = %v, wantNil %v", l, wantNil)
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
}

// fakeMetrics counts increments.
type fakeMetrics struct{ n int64 }

// Inc implements httpx.Metrics.
func (f *fakeMetrics) Inc(_ string, d int64) { f.n += d }

// TestApplyRateLimits verifies limiter, proxy, and metrics wiring.
//
// Parameters:
//   - t: the test handle.
func TestApplyRateLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, trusted, m := newRateLimitedHandler(ctx)
	checkRateLimitWiring(t, h, trusted, m)
	checkCreateRateLimit(t, h.Router(), m)
}

// newRateLimitedHandler applies a create limiter to a fresh handler.
//
// Parameters:
//   - ctx: the limiter cleanup context.
//
// Returns:
//   - *httpx.Handler: the configured handler.
//   - []netip.Prefix: the trusted proxy prefixes applied.
//   - *fakeMetrics: the metrics collector applied.
func newRateLimitedHandler(ctx context.Context) (*httpx.Handler, []netip.Prefix, *fakeMetrics) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cfg := &config.Config{
		CreateRate:      ratelimit.Rate{Count: 1, Per: time.Hour},
		RateBurst:       1,
		TrustedPrefixes: trusted,
	}
	m := &fakeMetrics{}
	h := httpx.New(nil, 1024, nil)
	applyRateLimits(ctx, h, cfg, m)
	return h, trusted, m
}

// checkRateLimitWiring verifies the handler received the expected settings.
//
// Parameters:
//   - t: the test handle.
//   - h: the handler to inspect.
//   - trusted: expected trusted proxy prefixes.
//   - m: expected metrics collector.
func checkRateLimitWiring(t *testing.T, h *httpx.Handler, trusted []netip.Prefix, m *fakeMetrics) {
	t.Helper()
	if h.CreateLimiter == nil || h.ReadLimiter != nil {
		t.Fatalf("create=%v read=%v, want create only", h.CreateLimiter, h.ReadLimiter)
	}
	if len(h.TrustedProxies) != 1 || h.TrustedProxies[0] != trusted[0] || h.Metrics != m {
		t.Fatal("trusted proxies or metrics not applied")
	}
}

// checkCreateRateLimit verifies the second create request is limited.
//
// Parameters:
//   - t: the test handle.
//   - router: the handler router.
//   - m: the metrics collector expected to record one limit.
func checkCreateRateLimit(t *testing.T, router http.Handler, m *fakeMetrics) {
	t.Helper()
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
