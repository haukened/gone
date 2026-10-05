package httpx

import (
	"log/slog"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/haukened/gone/v3/internal/metrics"
	"github.com/haukened/gone/v3/internal/ratelimit"
)

// RateLimiter decides whether a client identified by key may make a request.
// It is satisfied by *ratelimit.Limiter.
type RateLimiter interface {
	Allow(key netip.Prefix) (bool, time.Duration)
}

// Metrics is the counter interface used to record rate-limit rejections.
type Metrics interface {
	Inc(name string, delta int64)
}

// rateScope labels a limited route group for logs and metrics.
type rateScope struct {
	name    string
	counter string
}

var (
	scopeCreate = rateScope{name: "create", counter: metrics.CounterRateLimitedCreate}
	scopeRead   = rateScope{name: "read", counter: metrics.CounterRateLimitedRead}
)

// limit wraps next so requests are admitted only when l allows the client.
// It runs before the request body is read. A nil limiter disables limiting.
//
// Parameters:
//   - l: the limiter for this route group; nil returns next unchanged.
//   - scope: the route group label used in logs and metrics.
//   - warn: shared guard for the one-time untrusted X-Forwarded-For warning.
//   - next: the handler to protect.
//
// Returns:
//   - http.HandlerFunc: the protected handler.
func (h *Handler) limit(l RateLimiter, scope rateScope, warn *sync.Once, next http.HandlerFunc) http.HandlerFunc {
	if l == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		key, untrustedXFF := ratelimit.ClientKey(r.RemoteAddr, r.Header.Values("X-Forwarded-For"), h.TrustedProxies)
		if untrustedXFF {
			warn.Do(func() {
				slog.Warn("ignoring X-Forwarded-For from untrusted peer; set GONE_TRUSTED_PROXIES if gone runs behind a reverse proxy")
			})
		}
		if ok, wait := l.Allow(key); !ok {
			h.rejectRateLimited(w, r, scope, wait)
			return
		}
		next(w, r)
	}
}

// rejectRateLimited writes a 429 response with a Retry-After header and
// records the rejection. Client addresses are never logged.
//
// Parameters:
//   - w: the response writer.
//   - r: the rejected request.
//   - scope: the route group that rejected the request.
//   - wait: time until the client may retry.
func (h *Handler) rejectRateLimited(w http.ResponseWriter, r *http.Request, scope rateScope, wait time.Duration) {
	cid, _ := GetCorrelationID(r.Context())
	slog.Info("rate limited", "cid", cid, "scope", scope.name)
	if h.Metrics != nil {
		h.Metrics.Inc(scope.counter, 1)
	}
	w.Header().Set("Retry-After", retryAfterSeconds(wait))
	h.writeError(r.Context(), w, http.StatusTooManyRequests, "rate limited")
}

// maxRetryAfter caps the advertised retry delay.
const maxRetryAfter = 24 * time.Hour

// retryAfterSeconds formats wait as whole seconds for a Retry-After header,
// rounding up and clamping to the range [1s, 24h].
//
// Parameters:
//   - wait: time until a request will be admitted.
//
// Returns:
//   - string: the delay in decimal seconds.
func retryAfterSeconds(wait time.Duration) string {
	wait = min(max(wait, time.Second), maxRetryAfter)
	return strconv.Itoa(int(math.Ceil(wait.Seconds())))
}
