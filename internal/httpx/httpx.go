// Package httpx contains the HTTP delivery layer (net/http handlers) for the Gone service.
// It maps HTTP requests to the application service while enforcing validation, size
// limits, security headers, streaming semantics, and error translation.
// Handlers are split across files (create.go, consume.go, health.go, errors.go).
package httpx

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/i18n"
)

// ServicePort abstracts the subset of app.Service used by the HTTP layer.
// It is satisfied by *app.Service in production and mocked in tests.
type ServicePort interface {
	CreateSecret(ctx context.Context, ct io.Reader, size int64, version uint8, nonce string, ttl time.Duration) (app.Created, error)
	Claim(ctx context.Context, idStr, token string) (app.ClaimResult, error)
	Ack(ctx context.Context, idStr, token string) error
	Status(ctx context.Context, idStr, token string) (app.SecretStatus, error)
	Revoke(ctx context.Context, idStr, token string) error
}

// Handler wires HTTP endpoints to the application service.
// It is safe for concurrent use. Zero-value is not valid; construct via New.
type Handler struct {
	Service           ServicePort
	MaxBody           int64                       // mirror service.MaxBytes (defense-in-depth)
	Readiness         func(context.Context) error // optional readiness probe
	IndexTmpl         IndexRenderer               // optional renderer for index page
	AboutTmpl         AboutRenderer               // optional renderer for about page
	SecretTmpl        SecretRenderer              // optional renderer for secret consumption page
	ManageTmpl        SecretRenderer              // optional renderer for the sender manage page
	RequestTmpl       IndexRenderer               // optional renderer for the new-request page (/request)
	RequestDetailTmpl SecretRenderer              // optional renderer for one request (/request/{id})
	ReplyTmpl         IndexRenderer               // optional renderer for the reply page (/reply/{id})
	Requests          RequestPort                 // optional secret-request service; nil serves 404
	ErrorTmpl         IndexRenderer               // optional renderer for generic error pages (404, 500, etc.)
	Assets            http.FileSystem             // static assets filesystem (optional)
	MinTTL            time.Duration               // lower TTL bound (from config)
	MaxTTL            time.Duration               // upper TTL bound (from config)
	TTLOptions        []domain.TTLOption          // explicit configured TTL options
	I18n              *i18n.Bundle                // optional catalogs; nil renders every page in English

	CreateLimiter  RateLimiter    // optional limiter for POST /api/secret (nil disables)
	ReadLimiter    RateLimiter    // optional limiter for /api/secret/{id}[/status|/revoke] (nil disables)
	TrustedProxies []netip.Prefix // peers whose X-Forwarded-For is honored
	Metrics        Metrics        // optional counter sink for rate-limit rejections
}

// New returns a configured Handler.
// svc: application service port implementation.
// maxBody: maximum allowed request body size (0 disables extra check).
// readiness: optional probe function for /readyz (nil => always ready).
func New(svc ServicePort, maxBody int64, readiness func(context.Context) error) *Handler {
	return &Handler{Service: svc, MaxBody: maxBody, Readiness: readiness}
}

// Router constructs and returns an http.Handler with all routes mounted and
// security headers middleware applied. The API routes are rate limited when
// CreateLimiter or ReadLimiter is set; set them before calling Router.
func (h *Handler) Router() http.Handler {
	var xffWarn sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/about", h.handleAbout)
	mux.HandleFunc("/secret/", h.handleSecret) // expect /secret/{id}
	mux.HandleFunc("/manage/", h.handleManage) // expect /manage/{id}
	mux.HandleFunc("/request", h.handleRequestPage)
	mux.HandleFunc("/request/", h.handleRequestPage) // expect /request/{id}
	mux.HandleFunc("/reply/", h.handleReplyPage)     // expect /reply/{id}
	mux.HandleFunc("/api/secret", h.limit(h.CreateLimiter, scopeCreate, &xffWarn, h.handleCreateSecret))
	mux.HandleFunc("/api/secret/", h.limit(h.ReadLimiter, scopeRead, &xffWarn, h.handleConsumeSecret)) // /api/secret/{id}[/status|/revoke]
	mux.HandleFunc("/api/request", h.limit(h.CreateLimiter, scopeCreate, &xffWarn, h.handleCreateRequest))
	mux.HandleFunc("/api/request/", h.limitBy(h.requestLimit, &xffWarn, h.handleRequestRoute)) // /api/request/{id}[/reply|/status|/revoke]
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/readyz", h.handleReady)
	if h.Assets != nil {
		mux.Handle("/static/", http.StripPrefix("/static/", h.staticHandler()))
	}
	if h.I18n != nil {
		mux.Handle("GET "+i18n.CatalogPath, h.I18n.CatalogHandler())
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
		h.renderErrorPage(w, r, http.StatusNotFound, "error.notFound")
	})
	var root http.Handler = wrapped
	if h.I18n != nil {
		root = i18n.Middleware(h.I18n, wrapped)
	}
	// Order: security headers -> correlation ID -> locale -> fallback wrapper
	return h.secureHeaders(CorrelationIDMiddleware(root))
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
