package integration_test

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/httpx"
	"github.com/haukened/gone/internal/ratelimit"
	"github.com/haukened/gone/internal/store"
	"github.com/haukened/gone/internal/store/filesystem"
	"github.com/haukened/gone/internal/store/sqlite"
)

// fakeClock is a manually advanced clock shared by the limiters under test.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// now returns the current fake time.
//
// Returns:
//   - time.Time: the clock's current instant.
func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// advance moves the clock forward.
//
// Parameters:
//   - d: how far to move the clock.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// realClock satisfies app.Clock with the system time.
type realClock struct{}

// Now returns the current UTC time.
//
// Returns:
//   - time.Time: the current time.
func (realClock) Now() time.Time { return time.Now().UTC() }

// limits describes the limiters and proxy trust installed on a test server.
type limits struct {
	create, read ratelimit.Rate
	burst        int
	trusted      []netip.Prefix
}

// newServer starts the real HTTP stack backed by a temporary SQLite index and
// blob directory, with rate limiters driven by clk.
//
// Parameters:
//   - t: the test, used for cleanup and fatal errors.
//   - clk: the clock the limiters read.
//   - lim: rate-limit settings; a disabled rate installs no limiter.
//
// Returns:
//   - *httptest.Server: the running server.
func newServer(t *testing.T, clk *fakeClock, lim limits) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newHandler(t, clk, lim))
	t.Cleanup(srv.Close)
	return srv
}

// newHandler builds the real router over a temporary store, with rate
// limiters driven by clk.
//
// Parameters:
//   - t: the test, used for cleanup and fatal errors.
//   - clk: the clock the limiters read.
//   - lim: rate-limit settings; a disabled rate installs no limiter.
//
// Returns:
//   - http.Handler: the router.
func newHandler(t *testing.T, clk *fakeClock, lim limits) http.Handler {
	t.Helper()
	cfg := config.DefaultAppConfig
	h := httpx.New(newService(t, &cfg), cfg.MaxBytes, nil)
	h.MinTTL, h.MaxTTL = cfg.MinTTL, cfg.MaxTTL
	h.TrustedProxies = lim.trusted
	if lim.create.Enabled() {
		h.CreateLimiter = ratelimit.New(lim.create, lim.burst, ratelimit.WithClock(clk.now))
	}
	if lim.read.Enabled() {
		h.ReadLimiter = ratelimit.New(lim.read, lim.burst, ratelimit.WithClock(clk.now))
	}
	return h.Router()
}

// newService builds the real application service over a temporary SQLite
// index and blob directory.
//
// Parameters:
//   - t: the test, used for cleanup and fatal errors.
//   - cfg: configuration supplying size, TTL, and lease limits.
//
// Returns:
//   - *app.Service: the service.
func newService(t *testing.T, cfg *config.Config) *app.Service {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open(sqlite.DriverName, config.SQLiteDSNFor(dir))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cleanupClose(t, db)
	idx, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("init index: %v", err)
	}
	blobDir := filepath.Join(dir, "blobs")
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		t.Fatalf("mkdir blobs: %v", err)
	}
	blobs, err := filesystem.New(blobDir)
	if err != nil {
		t.Fatalf("blob store: %v", err)
	}
	return &app.Service{
		Store:      store.New(idx, blobs, realClock{}, cfg.InlineMaxBytes),
		Clock:      realClock{},
		MaxBytes:   cfg.MaxBytes,
		MinTTL:     cfg.MinTTL,
		MaxTTL:     cfg.MaxTTL,
		ClaimLease: cfg.ClaimLease,
	}
}

// do sends a request and returns the response, failing the test on
// transport errors. The response body is closed by a test cleanup.
//
// Parameters:
//   - t: the test.
//   - req: the request to send.
//
// Returns:
//   - *http.Response: the server's response.
func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	cleanupClose(t, resp.Body)
	return resp
}

// cleanupClose closes c at test cleanup time and reports close errors.
//
// Parameters:
//   - t: the test.
//   - c: the resource to close.
func cleanupClose(t *testing.T, c io.Closer) {
	t.Helper()
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Logf("close resource: %v", err)
		}
	})
}

// createReq builds a valid create request, optionally forwarded for xff.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - xff: an X-Forwarded-For value, or "" for none.
//
// Returns:
//   - *http.Request: the POST /api/secret request.
func createReq(t *testing.T, base, xff string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/api/secret", bytes.NewReader([]byte("ciphertext")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
	req.Header.Set("X-Gone-TTL", "10m")
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

// create posts a secret and returns the response.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//
// Returns:
//   - *http.Response: the server's response.
func create(t *testing.T, base string) *http.Response {
	t.Helper()
	return do(t, createReq(t, base, ""))
}
