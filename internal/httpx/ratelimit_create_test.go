package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/metrics"
)

func TestCreateRateLimited(t *testing.T) {
	logs := captureLogs(t)
	svc := &recordingService{}
	lim := &stubLimiter{n: 1, wait: 1500 * time.Millisecond}
	m := &countingMetrics{}
	h := &Handler{Service: svc, MaxBody: 1024, CreateLimiter: lim, Metrics: m}

	w, body := exerciseCreateLimit(t, h.Router())
	assertCreateLimitedResponse(t, w)
	assertCreateLimitSideEffects(t, svc, m, lim, body, logs.String())
}

// exerciseCreateLimit sends one allowed create request followed by a limited request.
// It takes t for failures and router for dispatch, and returns the limited response and tracked body.
func exerciseCreateLimit(t *testing.T, router http.Handler) (*httptest.ResponseRecorder, *trackingBody) {
	t.Helper()
	first := httptest.NewRecorder()
	router.ServeHTTP(first, createRequest(strings.NewReader("cipher")))
	if first.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want 201", first.Code)
	}
	body := &trackingBody{r: strings.NewReader("cipher")}
	req := createRequest(body)
	req.Header.Set(CorrelationIDHeader, "11111111-2222-4333-8444-555555555555")
	second := httptest.NewRecorder()
	router.ServeHTTP(second, req)
	return second, body
}

// assertCreateLimitedResponse verifies status, headers, and JSON for a limited create response.
// It takes t for failures and w as the response to inspect.
func assertCreateLimitedResponse(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
	assertCreateLimitJSON(t, w)
	assertCreateLimitHeaders(t, w)
}

// assertCreateLimitJSON verifies the rate-limited JSON error body.
// It takes t for failures and w as the response to inspect.
func assertCreateLimitJSON(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var resp struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Error != "rate limited" {
		t.Fatalf("body = %q (err %v)", w.Body.String(), err)
	}
}

// assertCreateLimitHeaders verifies security and correlation headers on a limited response.
// It takes t for failures and w as the response to inspect.
func assertCreateLimitHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing on 429")
	}
	if w.Header().Get(CorrelationIDHeader) == "" {
		t.Fatal("correlation ID missing on 429")
	}
}

// assertCreateLimitSideEffects verifies state changed only as expected after create rate limiting.
// It takes t for failures plus the service, metrics, limiter, body tracker, and captured log output.
func assertCreateLimitSideEffects(t *testing.T, svc *recordingService, m *countingMetrics, lim *stubLimiter, body *trackingBody, out string) {
	t.Helper()
	if body.read {
		t.Fatal("request body was read before the rate limit check")
	}
	if svc.create != 1 {
		t.Fatalf("service create calls = %d, want 1", svc.create)
	}
	assertCreateLimitMetricsAndKey(t, m, lim)
	assertCreateLimitLogs(t, out)
}

// assertCreateLimitMetricsAndKey verifies rate-limit metrics and peer key selection.
// It takes t for failures plus the metrics and limiter to inspect.
func assertCreateLimitMetricsAndKey(t *testing.T, m *countingMetrics, lim *stubLimiter) {
	t.Helper()
	if m.counts[metrics.CounterRateLimitedCreate] != 1 || m.counts[metrics.CounterRateLimitedRead] != 0 {
		t.Fatalf("metrics = %v", m.counts)
	}
	if lim.keys[0] != netip.MustParsePrefix("192.0.2.10/32") {
		t.Fatalf("limiter key = %s", lim.keys[0])
	}
}

// assertCreateLimitLogs verifies logs contain scope and correlation ID without client IP leakage.
// It takes t for failures and out as the captured log text.
func assertCreateLimitLogs(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, `"scope":"create"`) || !strings.Contains(out, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("log missing scope or cid: %s", out)
	}
	if strings.Contains(out, "192.0.2.10") {
		t.Fatalf("client address leaked into logs: %s", out)
	}
}
