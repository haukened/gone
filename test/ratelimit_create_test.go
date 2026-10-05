package integration_test

import (
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/ratelimit"
)

// TestCreateBurstThenRecovery verifies create burst exhaustion and refill.
//
// Parameters:
//   - t: the test handle.
func TestCreateBurstThenRecovery(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{create: ratelimit.Rate{Count: 2, Per: time.Minute}, burst: 2})

	for i := 0; i < 2; i++ {
		if got := createStatus(t, srv.URL); got != http.StatusCreated {
			t.Fatalf("create %d status = %d, want 201", i, got)
		}
	}
	assertCreateLimited(t, srv.URL)

	clk.advance(30 * time.Second)
	if got := createStatus(t, srv.URL); got != http.StatusCreated {
		t.Fatalf("after refill status = %d, want 201", got)
	}
	assertCreateLimited(t, srv.URL)
}

// TestReadAndCreateBudgetsAreIndependent verifies independent limiter buckets.
//
// Parameters:
//   - t: the test handle.
func TestReadAndCreateBudgetsAreIndependent(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{
		create: ratelimit.Rate{Count: 1, Per: time.Hour},
		read:   ratelimit.Rate{Count: 1, Per: time.Hour},
		burst:  1,
	})
	if got := createStatus(t, srv.URL); got != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", got)
	}
	assertCreateLimited(t, srv.URL)

	missing := "0123456789abcdef0123456789abcdef"
	if got := readRateStatus(t, srv.URL, missing, ""); got == http.StatusTooManyRequests {
		t.Fatal("first read was rate limited by the create budget")
	}
	assertReadLimited(t, srv.URL, missing, "")
}

// TestTrustedProxySeparatesClients verifies trusted X-Forwarded-For handling.
//
// Parameters:
//   - t: the test handle.
func TestTrustedProxySeparatesClients(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{
		create:  ratelimit.Rate{Count: 1, Per: time.Hour},
		burst:   1,
		trusted: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")},
	})
	if got := forwardedCreateStatus(t, srv.URL, "192.0.2.10"); got != http.StatusCreated {
		t.Fatalf("client A status = %d, want 201", got)
	}
	assertForwardedCreateLimited(t, srv.URL, "192.0.2.10")
	if got := forwardedCreateStatus(t, srv.URL, "198.51.100.20"); got != http.StatusCreated {
		t.Fatalf("client B status = %d, want 201", got)
	}
	if got := forwardedCreateStatus(t, srv.URL, "2001:db8::1"); got != http.StatusCreated {
		t.Fatalf("client C status = %d, want 201", got)
	}
	assertForwardedCreateLimited(t, srv.URL, "2001:db8::2")
}
