package httpx

import (
	"bytes"
	"net/http"
	"time"

	"github.com/haukened/gone/v3/internal/brand"
)

// svgCSP replaces the page policy on an SVG logo, in case someone opens the
// file directly: no scripts, no outside loads, and a sandboxed document.
const svgCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox"

// brandLogo and brandAccent pick an asset from the branding, which may be
// nil.
func brandLogo(b *brand.Brand) *brand.Asset {
	if b == nil {
		return nil
	}
	return b.Logo
}

func brandAccent(b *brand.Brand) *brand.Asset {
	if b == nil {
		return nil
	}
	return b.Accent
}

// brandAsset returns the handler for one branding asset (GET /brand/logo or
// /brand/accent.css). Other methods get 405; when the asset isn't
// configured, GET falls through to the 404 page. A request carrying the
// asset's current ?v= may be cached for a year, like the fingerprinted
// static files; any other URL gets the static files' short lifetime.
//
// Parameters:
//   - pick: selects the asset from h.Brand.
//
// Returns the handler.
func (h *Handler) brandAsset(pick func(*brand.Brand) *brand.Asset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := pick(h.Brand)
		if !h.allowProbeMethod(w, r) || a == nil {
			return
		}
		if a.Type == "image/svg+xml" {
			w.Header().Set("Content-Security-Policy", svgCSP)
		}
		cache := "public, max-age=300"
		if r.URL.Query().Get("v") == a.Version {
			cache = "public, max-age=31536000, immutable"
		}
		w.Header().Set("Cache-Control", cache)
		w.Header().Del("Pragma")
		w.Header().Set("Content-Type", a.Type)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(a.Data))
	}
}
