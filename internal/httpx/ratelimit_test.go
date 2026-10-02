package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/metrics"
)

// stubLimiter allows the first n requests and records the keys it sees.
type stubLimiter struct {
	mu    sync.Mutex
	n     int
	wait  time.Duration
	keys  []netip.Prefix
	calls int
}

// Allow implements RateLimiter.
func (s *stubLimiter) Allow(key netip.Prefix) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.keys = append(s.keys, key)
	if s.calls <= s.n {
		return true, 0
	}
	return false, s.wait
}

// countingMetrics records counter increments.
type countingMetrics struct {
	mu     sync.Mutex
	counts map[string]int64
}

// Inc implements Metrics.
func (c *countingMetrics) Inc(name string, delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int64{}
	}
	c.counts[name] += delta
}

// recordingService counts service calls.
type recordingService struct {
	mu     sync.Mutex
	create int
	claim  int
}

// CreateSecret implements ServicePort.
func (s *recordingService) CreateSecret(_ context.Context, ct io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (domain.SecretID, time.Time, error) {
	s.mu.Lock()
	s.create++
	s.mu.Unlock()
	_, _ = io.Copy(io.Discard, ct)
	return domain.SecretID(strings.Repeat("a", 32)), time.Unix(1000, 0), nil
}

// Claim implements ServicePort.
func (s *recordingService) Claim(context.Context, string, string) (app.ClaimResult, error) {
	s.mu.Lock()
	s.claim++
	s.mu.Unlock()
	return app.ClaimResult{}, app.ErrNotFound
}

// Ack implements ServicePort.
func (s *recordingService) Ack(context.Context, string, string) error { return nil }

// trackingBody reports whether it was read.
type trackingBody struct {
	r    io.Reader
	read bool
}

// Read implements io.Reader.
func (b *trackingBody) Read(p []byte) (int, error) {
	b.read = true
	return b.r.Read(p)
}

// createRequest builds a valid POST /api/secret request.
func createRequest(body io.Reader) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/secret", body)
	req.Header.Set("Content-Length", "6")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "n1")
	req.Header.Set("X-Gone-TTL", "5m")
	req.RemoteAddr = "192.0.2.10:4000"
	return req
}

// captureLogs redirects the default slog logger for the duration of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func TestCreateRateLimited(t *testing.T) {
	logs := captureLogs(t)
	svc := &recordingService{}
	lim := &stubLimiter{n: 1, wait: 1500 * time.Millisecond}
	m := &countingMetrics{}
	h := &Handler{Service: svc, MaxBody: 1024, CreateLimiter: lim, Metrics: m}
	router := h.Router()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, createRequest(strings.NewReader("cipher")))
	if w.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want 201", w.Code)
	}

	body := &trackingBody{r: strings.NewReader("cipher")}
	req := createRequest(body)
	req.Header.Set(CorrelationIDHeader, "11111111-2222-4333-8444-555555555555")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
	var resp struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Error != "rate limited" {
		t.Fatalf("body = %q (err %v)", w.Body.String(), err)
	}
	if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing on 429")
	}
	if w.Header().Get(CorrelationIDHeader) == "" {
		t.Fatal("correlation ID missing on 429")
	}
	if body.read {
		t.Fatal("request body was read before the rate limit check")
	}
	if svc.create != 1 {
		t.Fatalf("service create calls = %d, want 1", svc.create)
	}
	if m.counts[metrics.CounterRateLimitedCreate] != 1 || m.counts[metrics.CounterRateLimitedRead] != 0 {
		t.Fatalf("metrics = %v", m.counts)
	}
	if lim.keys[0] != netip.MustParsePrefix("192.0.2.10/32") {
		t.Fatalf("limiter key = %s", lim.keys[0])
	}
	out := logs.String()
	if !strings.Contains(out, `"scope":"create"`) || !strings.Contains(out, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("log missing scope or cid: %s", out)
	}
	if strings.Contains(out, "192.0.2.10") {
		t.Fatalf("client address leaked into logs: %s", out)
	}
}

func TestReadRateLimited(t *testing.T) {
	svc := &recordingService{}
	lim := &stubLimiter{n: 0, wait: 0}
	m := &countingMetrics{}
	h := &Handler{Service: svc, ReadLimiter: lim, Metrics: m}
	router := h.Router()

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/secret/"+strings.Repeat("a", 32), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("%s status = %d, want 429", method, w.Code)
		}
		if got := w.Header().Get("Retry-After"); got != "1" {
			t.Fatalf("Retry-After = %q, want 1", got)
		}
	}
	if svc.claim != 0 {
		t.Fatalf("service reached %d times", svc.claim)
	}
	if m.counts[metrics.CounterRateLimitedRead] != 2 {
		t.Fatalf("metrics = %v", m.counts)
	}
}

func TestRateLimitScopesAreIndependent(t *testing.T) {
	svc := &recordingService{}
	create := &stubLimiter{n: 0}
	h := &Handler{Service: svc, MaxBody: 1024, CreateLimiter: create}
	router := h.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/secret/"+strings.Repeat("a", 32), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Fatal("read route limited by create limiter")
	}
	if svc.claim != 1 {
		t.Fatalf("claim calls = %d, want 1", svc.claim)
	}
	if create.calls != 0 {
		t.Fatalf("create limiter consulted %d times for read", create.calls)
	}
}

func TestUnlimitedRoutes(t *testing.T) {
	create, read := &stubLimiter{}, &stubLimiter{}
	h := &Handler{Service: &recordingService{}, CreateLimiter: create, ReadLimiter: read}
	router := h.Router()
	for _, path := range []string{"/healthz", "/readyz", "/", "/about", "/secret/abc", "/api/other"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("%s was rate limited", path)
		}
	}
	if create.calls+read.calls != 0 {
		t.Fatalf("limiters consulted for unlimited routes: create=%d read=%d", create.calls, read.calls)
	}
}

func TestNilLimiterPassthrough(t *testing.T) {
	svc := &recordingService{}
	h := &Handler{Service: svc, MaxBody: 1024}
	router := h.Router()
	for i := 0; i < 50; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, createRequest(strings.NewReader("cipher")))
		if w.Code != http.StatusCreated {
			t.Fatalf("request %d status = %d", i, w.Code)
		}
	}
}

func TestRateLimitedWithoutMetrics(t *testing.T) {
	h := &Handler{Service: &recordingService{}, CreateLimiter: &stubLimiter{}}
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, createRequest(strings.NewReader("cipher")))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func TestTrustedProxyKeying(t *testing.T) {
	lim := &stubLimiter{n: 10}
	h := &Handler{
		Service:        &recordingService{},
		ReadLimiter:    lim,
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/secret/"+strings.Repeat("a", 32), nil)
	req.RemoteAddr = "10.1.1.1:443"
	req.Header.Add("X-Forwarded-For", "203.0.113.9, 198.51.100.4")
	h.Router().ServeHTTP(httptest.NewRecorder(), req)
	if want := netip.MustParsePrefix("198.51.100.4/32"); lim.keys[0] != want {
		t.Fatalf("key = %s, want %s", lim.keys[0], want)
	}
}

func TestUntrustedXFFWarnsOnce(t *testing.T) {
	logs := captureLogs(t)
	lim := &stubLimiter{n: 10}
	h := &Handler{Service: &recordingService{}, MaxBody: 1024, CreateLimiter: lim, ReadLimiter: lim}
	router := h.Router()
	for i := 0; i < 3; i++ {
		req := createRequest(strings.NewReader("cipher"))
		req.Header.Set("X-Forwarded-For", "198.51.100.4")
		router.ServeHTTP(httptest.NewRecorder(), req)
		get := httptest.NewRequest(http.MethodGet, "/api/secret/"+strings.Repeat("a", 32), nil)
		get.Header.Set("X-Forwarded-For", "198.51.100.4")
		router.ServeHTTP(httptest.NewRecorder(), get)
	}
	if n := strings.Count(logs.String(), "untrusted peer"); n != 1 {
		t.Fatalf("warning logged %d times, want 1", n)
	}
	if strings.Contains(logs.String(), "198.51.100.4") {
		t.Fatal("forwarded address leaked into logs")
	}
	if lim.keys[0] != netip.MustParsePrefix("192.0.2.10/32") {
		t.Fatalf("key = %s, want peer address", lim.keys[0])
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	tests := []struct {
		wait time.Duration
		want string
	}{
		{wait: -time.Second, want: "1"},
		{wait: 0, want: "1"},
		{wait: time.Millisecond, want: "1"},
		{wait: time.Second, want: "1"},
		{wait: 1001 * time.Millisecond, want: "2"},
		{wait: time.Minute, want: "60"},
		{wait: 48 * time.Hour, want: "86400"},
	}
	for _, tc := range tests {
		if got := retryAfterSeconds(tc.wait); got != tc.want {
			t.Fatalf("retryAfterSeconds(%v) = %s, want %s", tc.wait, got, tc.want)
		}
	}
}
