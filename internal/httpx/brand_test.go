package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/brand"
	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/httpx"
)

// brandRouter returns a router whose branding has an SVG logo and an accent.
func brandRouter(t *testing.T) (http.Handler, *brand.Brand) {
	t.Helper()
	logo := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(logo, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := brand.Load(&config.Config{BrandLogo: logo, AccentLight: "#1d4ed8", AccentDark: "#93c5fd"})
	if err != nil {
		t.Fatal(err)
	}
	h := httpx.New(failingService{}, 1024, nil)
	h.Brand = b
	return h.Router(), b
}

func get(h http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestBrandAssets(t *testing.T) {
	h, b := brandRouter(t)
	tests := []struct {
		target, ctype, cache, csp string
		body                      []byte
	}{
		{b.LogoURL(), "image/svg+xml", "public, max-age=31536000, immutable", "sandbox", b.Logo.Data},
		{brand.LogoPath, "image/svg+xml", "public, max-age=300", "sandbox", b.Logo.Data},
		{b.AccentURL(), "text/css; charset=utf-8", "public, max-age=31536000, immutable", "style-src 'self'", b.Accent.Data},
		{brand.AccentPath + "?v=stale", "text/css; charset=utf-8", "public, max-age=300", "style-src 'self'", b.Accent.Data},
	}
	for _, tc := range tests {
		w := get(h, http.MethodGet, tc.target)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != tc.ctype || w.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: %d %q %q", tc.target, w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"))
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), tc.csp) || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: CSP %q, nosniff %q", tc.target, w.Header().Get("Content-Security-Policy"), w.Header().Get("X-Content-Type-Options"))
		}
		if w.Body.String() != string(tc.body) || w.Header().Get("Pragma") != "" {
			t.Errorf("%s: body %q, Pragma %q", tc.target, w.Body.String(), w.Header().Get("Pragma"))
		}
	}
}

func TestBrandAssetMethods(t *testing.T) {
	h, _ := brandRouter(t)
	if w := get(h, http.MethodHead, brand.LogoPath); w.Code != http.StatusOK {
		t.Fatalf("HEAD = %d", w.Code)
	}
	w := get(h, http.MethodPost, brand.AccentPath)
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST = %d Allow %q", w.Code, w.Header().Get("Allow"))
	}
}

func TestBrandAssetsUnset(t *testing.T) {
	for _, b := range []*brand.Brand{nil, {Name: "Acme"}} {
		h := httpx.New(failingService{}, 1024, nil)
		h.Brand = b
		for _, p := range []string{brand.LogoPath, brand.AccentPath} {
			if w := get(h.Router(), http.MethodGet, p); w.Code != http.StatusNotFound {
				t.Errorf("brand %+v: %s = %d, want 404", b, p, w.Code)
			}
		}
	}
}
