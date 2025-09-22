// Package httpx contains the HTTP delivery layer (net/http handlers) for the Gone service.
// It maps HTTP requests to the application service while enforcing validation, size
// limits, security headers, streaming semantics, and error translation.
// Handlers are split across files (create.go, consume.go, health.go, errors.go).
package httpx

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
)

// ServicePort abstracts the subset of app.Service used by the HTTP layer.
// It is satisfied by *app.Service in production and mocked in tests.
type ServicePort interface {
	CreateSecret(ctx context.Context, ct io.Reader, size int64, version uint8, nonce string, ttl time.Duration) (id domain.SecretID, expiresAt time.Time, err error)
	Consume(ctx context.Context, idStr string) (app.Meta, io.ReadCloser, int64, error)
}

// Handler wires HTTP endpoints to the application service.
// It is safe for concurrent use. Zero-value is not valid; construct via New.
type Handler struct {
	Service   ServicePort
	MaxBody   int64                       // mirror service.MaxBytes (defense-in-depth)
	Readiness func(context.Context) error // optional readiness probe
}

// New returns a configured Handler.
// svc: application service port implementation.
// maxBody: maximum allowed request body size (0 disables extra check).
// readiness: optional probe function for /readyz (nil => always ready).
func New(svc ServicePort, maxBody int64, readiness func(context.Context) error) *Handler {
	return &Handler{Service: svc, MaxBody: maxBody, Readiness: readiness}
}

// Router constructs and returns an http.Handler with all routes mounted and
// security headers middleware applied.
func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/secret", h.handleCreateSecret)
	mux.HandleFunc("/api/secret/", h.handleConsumeSecret) // expect /api/secret/{id}
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/readyz", h.handleReady)
	return h.secureHeaders(mux)
}

// secureHeaders middleware adds standard security & cache control headers.
func (h *Handler) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}
