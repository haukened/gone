package httpx

import (
	"bytes"
	"context"
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
	manage int
}

// CreateSecret implements ServicePort.
func (s *recordingService) CreateSecret(_ context.Context, ct io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
	s.mu.Lock()
	s.create++
	s.mu.Unlock()
	if _, err := io.Copy(io.Discard, ct); err != nil {
		return app.Created{}, err
	}
	return app.Created{ID: domain.SecretID(strings.Repeat("a", 32)), ExpiresAt: time.Unix(1000, 0)}, nil
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

// Status implements ServicePort.
func (s *recordingService) Status(context.Context, string, string) (app.SecretStatus, error) {
	s.mu.Lock()
	s.manage++
	s.mu.Unlock()
	return app.SecretStatus{}, app.ErrNotFound
}

// Revoke implements ServicePort.
func (s *recordingService) Revoke(context.Context, string, string) error {
	s.mu.Lock()
	s.manage++
	s.mu.Unlock()
	return app.ErrNotFound
}

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
	req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
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
