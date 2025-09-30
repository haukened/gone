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
	Service    ServicePort
	MaxBody    int64                       // mirror service.MaxBytes (defense-in-depth)
	Readiness  func(context.Context) error // optional readiness probe
	IndexTmpl  IndexRenderer               // optional renderer for index page
	AboutTmpl  AboutRenderer               // optional renderer for about page
	SecretTmpl SecretRenderer              // optional renderer for secret consumption page
	ErrorTmpl  IndexRenderer               // optional renderer for generic error pages (404, 500, etc.)
	Assets     http.FileSystem             // static assets filesystem (optional)
	MinTTL     time.Duration               // lower TTL bound (from config)
	MaxTTL     time.Duration               // upper TTL bound (from config)
	TTLOptions []domain.TTLOption          // explicit configured TTL options
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
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/about", h.handleAbout)
	mux.HandleFunc("/secret/", h.handleSecret) // expect /secret/{id}
	mux.HandleFunc("/api/secret", h.handleCreateSecret)
	mux.HandleFunc("/api/secret/", h.handleConsumeSecret) // expect /api/secret/{id}
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/readyz", h.handleReady)
	if h.Assets != nil {
		mux.Handle("/static/", http.StripPrefix("/static/", h.staticHandler()))
	}
	// We can't set a NotFoundHandler on net/http ServeMux; instead wrap the constructed mux
	// with a fallback that checks for 404 responses after attempting routing.
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use a ResponseRecorder-like shim to detect if a handler wrote anything.
		rw := &probeWriter{ResponseWriter: w}
		mux.ServeHTTP(rw, r)
		if rw.wroteHeader { // some handler handled it
			return
		}
		// No handler matched: choose JSON vs HTML based on path prefix.
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			h.writeError(r.Context(), w, http.StatusNotFound, "not found")
			return
		}
		h.renderErrorPage(w, r, http.StatusNotFound, "Not Found", "The page you requested was not found.")
	})
	// Order: correlation ID -> security headers -> fallback wrapper
	return h.secureHeaders(CorrelationIDMiddleware(wrapped))
}

// probeWriter records whether a downstream handler wrote headers/body.
type probeWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (p *probeWriter) WriteHeader(code int) {
	p.wroteHeader = true
	p.ResponseWriter.WriteHeader(code)
}

func (p *probeWriter) Write(b []byte) (int, error) {
	p.wroteHeader = true
	return p.ResponseWriter.Write(b)
}

// secureHeaders middleware adds standard security & cache control headers.
func (h *Handler) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Default: deny everything, then allow only self scripts/styles/images.
		// Avoid inline scripts/styles to keep a strong CSP.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Cache defaults per route: index handler will override to no-store; static handler sets long-lived.
		if ct := w.Header().Get("Content-Type"); ct == "" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
