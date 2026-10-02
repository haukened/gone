package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
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
	srv := httptest.NewServer(h.Router())
	t.Cleanup(srv.Close)
	return srv
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
	t.Cleanup(func() { _ = db.Close() })
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
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
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

// read issues GET /api/secret/{id} with an optional claim token.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret id.
//   - claim: a claim token, or "" for none.
//
// Returns:
//   - *http.Response: the server's response.
func read(t *testing.T, base, id, claim string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/api/secret/"+id, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if claim != "" {
		req.Header.Set(httpx.HeaderClaim, claim)
	}
	return do(t, req)
}

// assertLimited checks that resp is a 429 with a positive whole-second
// Retry-After header.
//
// Parameters:
//   - t: the test.
//   - resp: the response to check.
func assertLimited(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || secs < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer", resp.Header.Get("Retry-After"))
	}
}

func TestCreateBurstThenRecovery(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{create: ratelimit.Rate{Count: 2, Per: time.Minute}, burst: 2})

	for i := 0; i < 2; i++ {
		if got := create(t, srv.URL).StatusCode; got != http.StatusCreated {
			t.Fatalf("create %d status = %d, want 201", i, got)
		}
	}
	assertLimited(t, create(t, srv.URL))

	clk.advance(30 * time.Second)
	if got := create(t, srv.URL).StatusCode; got != http.StatusCreated {
		t.Fatalf("after refill status = %d, want 201", got)
	}
	assertLimited(t, create(t, srv.URL))
}

func TestReadAndCreateBudgetsAreIndependent(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{
		create: ratelimit.Rate{Count: 1, Per: time.Hour},
		read:   ratelimit.Rate{Count: 1, Per: time.Hour},
		burst:  1,
	})
	if got := create(t, srv.URL).StatusCode; got != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", got)
	}
	assertLimited(t, create(t, srv.URL))

	missing := "0123456789abcdef0123456789abcdef"
	if got := read(t, srv.URL, missing, "").StatusCode; got == http.StatusTooManyRequests {
		t.Fatal("first read was rate limited by the create budget")
	}
	assertLimited(t, read(t, srv.URL, missing, ""))
}

func TestTrustedProxySeparatesClients(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{
		create:  ratelimit.Rate{Count: 1, Per: time.Hour},
		burst:   1,
		trusted: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")},
	})
	if got := do(t, createReq(t, srv.URL, "192.0.2.10")).StatusCode; got != http.StatusCreated {
		t.Fatalf("client A status = %d, want 201", got)
	}
	assertLimited(t, do(t, createReq(t, srv.URL, "192.0.2.10")))
	if got := do(t, createReq(t, srv.URL, "198.51.100.20")).StatusCode; got != http.StatusCreated {
		t.Fatalf("client B status = %d, want 201", got)
	}
	// IPv6 clients are grouped by /64, so another address in the same /64
	// shares a budget.
	if got := do(t, createReq(t, srv.URL, "2001:db8::1")).StatusCode; got != http.StatusCreated {
		t.Fatalf("client C status = %d, want 201", got)
	}
	assertLimited(t, do(t, createReq(t, srv.URL, "2001:db8::2")))
}

// createID creates a secret and returns its ID.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//
// Returns:
//   - string: the new secret's ID.
func createID(t *testing.T, base string) string {
	t.Helper()
	resp := create(t, base)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil || created.ID == "" {
		t.Fatalf("decode create response: %v (id %q)", err, created.ID)
	}
	return created.ID
}

// claimToken claims a secret and returns the claim token.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//
// Returns:
//   - string: the claim token from the response.
func claimToken(t *testing.T, base, id string) string {
	t.Helper()
	resp := read(t, base, id, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("claim status = %d, want 200", resp.StatusCode)
	}
	token := resp.Header.Get(httpx.HeaderClaim)
	if token == "" {
		t.Fatal("claim response missing claim token")
	}
	return token
}

// ack acknowledges a claimed secret.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - token: the claim token.
//
// Returns:
//   - int: the response status code.
func ack(t *testing.T, base, id, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, base+"/api/secret/"+id, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(httpx.HeaderClaim, token)
	return do(t, req).StatusCode
}

func TestFullFlowUnderDefaults(t *testing.T) {
	cfg := config.DefaultAppConfig
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{create: cfg.CreateRate, read: cfg.ReadRate, burst: cfg.RateBurst})

	id := createID(t, srv.URL)
	token := claimToken(t, srv.URL, id)
	if got := ack(t, srv.URL, id, token); got != http.StatusNoContent {
		t.Fatalf("ack status = %d, want 204", got)
	}
	if got := read(t, srv.URL, id, "").StatusCode; got != http.StatusNotFound {
		t.Fatalf("read after ack status = %d, want 404", got)
	}
}
