package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/metrics"
)

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

func TestManageRoutesReadRateLimited(t *testing.T) {
	svc := &recordingService{}
	lim := &stubLimiter{n: 0}
	m := &countingMetrics{}
	h := &Handler{Service: svc, ReadLimiter: lim, Metrics: m}
	router := h.Router()
	id := strings.Repeat("a", 32)
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/api/secret/" + id + "/status"},
		{http.MethodPost, "/api/secret/" + id + "/revoke"},
	} {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		req.Header.Set(HeaderManage, "token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("%s %s status = %d, want 429", rt.method, rt.path, w.Code)
		}
	}
	if svc.manage != 0 {
		t.Fatalf("service reached %d times", svc.manage)
	}
	if lim.calls != 2 || m.counts[metrics.CounterRateLimitedRead] != 2 {
		t.Fatalf("limiter calls = %d, metrics = %v", lim.calls, m.counts)
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
