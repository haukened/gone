package main

import (
	"context"
	"log/slog"

	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/httpx"
	"github.com/haukened/gone/internal/ratelimit"
)

// applyRateLimits configures per-client rate limiting on h and starts the
// limiters' background sweeps. Call it before h.Router.
//
// Parameters:
//   - ctx: lifetime of the sweep loops.
//   - h: the handler to configure.
//   - cfg: configuration supplying rates, burst, and trusted proxies.
//   - m: counter sink for rejections; may be nil.
func applyRateLimits(ctx context.Context, h *httpx.Handler, cfg *config.Config, m httpx.Metrics) {
	h.CreateLimiter = startLimiter(ctx, "create", cfg.CreateRate, cfg.RateBurst)
	h.ReadLimiter = startLimiter(ctx, "read", cfg.ReadRate, cfg.RateBurst)
	h.TrustedProxies = cfg.TrustedPrefixes
	h.Metrics = m
}

// startLimiter builds a limiter for one route group and starts its sweep
// loop. A disabled rate yields an untyped nil so the handler skips limiting.
//
// Parameters:
//   - ctx: lifetime of the sweep loop.
//   - scope: route group label for logs.
//   - rate: per-client budget; disabled when !rate.Enabled().
//   - burst: bucket capacity.
//
// Returns:
//   - httpx.RateLimiter: the running limiter, or nil when disabled.
func startLimiter(ctx context.Context, scope string, rate ratelimit.Rate, burst int) httpx.RateLimiter {
	if !rate.Enabled() {
		slog.Warn("rate limiting disabled", "scope", scope)
		return nil
	}
	l := ratelimit.New(rate, burst)
	go l.Run(ctx, ratelimit.DefaultSweepInterval)
	slog.Info("rate limiting enabled", "scope", scope, "count", rate.Count, "per", rate.Per.String(), "burst", burst)
	return l
}
