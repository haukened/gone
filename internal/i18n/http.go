package i18n

import (
	"bytes"
	"context"
	"net/http"
	"strings"
)

// ctxKey is the request-context key for the chosen locale.
type ctxKey struct{}

// Middleware stores the request's locale (see Bundle.Match) in its context
// for FromContext. It sets no headers; pages add Content-Language and Vary
// when they render (see PageHeaders).
//
// Parameters:
//   - b: the loaded catalogs.
//   - next: the handler to wrap.
//
// Returns:
//   - http.Handler: the wrapped handler.
func Middleware(b *Bundle, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saved := ""
		if c, err := r.Cookie(CookieName); err == nil {
			saved = c.Value
		}
		tag := b.Match(saved, r.Header.Get("Accept-Language"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, tag)))
	})
}

// FromContext returns the locale Middleware stored, or the base locale.
func FromContext(ctx context.Context) string {
	if tag, ok := ctx.Value(ctxKey{}).(string); ok {
		return tag
	}
	return BaseLocale
}

// PageHeaders marks a page response as being in tag and as varying by the
// inputs Match reads.
//
// Parameters:
//   - h: the response headers.
//   - tag: the page's locale.
func PageHeaders(h http.Header, tag string) {
	h.Set("Content-Language", tag)
	h.Add("Vary", "Cookie, Accept-Language")
}

// CatalogPath is the URL prefix CatalogHandler serves under.
const CatalogPath = "/i18n/"

// CatalogURL returns the cache-busting URL of tag's browser catalog.
func (b *Bundle) CatalogURL(tag string) string {
	_, sum := b.Catalog(tag)
	return CatalogPath + tag + ".json?v=" + sum
}

// CatalogHandler serves each loaded locale's catalog at
// /i18n/<tag>.json. A request carrying the current hash (?v=) may be cached
// for a year; others for five minutes.
//
// Returns:
//   - http.Handler: the handler, to mount at CatalogPath.
func (b *Bundle) CatalogHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, CatalogPath), ".json")
		body, sum := b.Catalog(tag)
		if body == nil || !strings.HasSuffix(r.URL.Path, ".json") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("v") == sum {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = bytes.NewReader(body).WriteTo(w)
	})
}
