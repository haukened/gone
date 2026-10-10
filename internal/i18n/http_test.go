package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware(t *testing.T) {
	b := loadTest(t, Options{}, nil)
	var got string
	h := Middleware(b, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = FromContext(r.Context()) }))
	cases := []struct{ cookie, accept, want string }{
		{"", "", "en"},
		{"", "es-ES", "es"},
		{"en", "es-ES", "en"},
		{"nope", "es", "es"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.cookie})
		}
		req.Header.Set("Accept-Language", tc.accept)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != tc.want {
			t.Errorf("cookie %q accept %q: %q, want %q", tc.cookie, tc.accept, got, tc.want)
		}
	}
	if FromContext(context.Background()) != BaseLocale {
		t.Fatal("FromContext default")
	}
}

func TestPageHeaders(t *testing.T) {
	h := http.Header{}
	h.Add("Vary", "Accept-Encoding")
	PageHeaders(h, "es")
	if h.Get("Content-Language") != "es" || len(h.Values("Vary")) != 2 || h.Values("Vary")[1] != "Cookie, Accept-Language" {
		t.Fatalf("headers = %v", h)
	}
}

func TestCatalogHandler(t *testing.T) {
	b := loadTest(t, Options{}, nil)
	_, sum := b.Catalog("es")
	h := b.CatalogHandler()
	cases := []struct {
		path, cache string
		status      int
	}{
		{"/i18n/es.json?v=" + sum, "public, max-age=31536000, immutable", http.StatusOK},
		{"/i18n/es.json", "public, max-age=300", http.StatusOK},
		{"/i18n/es.json?v=old", "public, max-age=300", http.StatusOK},
		{"/i18n/fr.json", "", http.StatusNotFound},
		{"/i18n/es", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rw.Code != tc.status || rw.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: %d %q", tc.path, rw.Code, rw.Header().Get("Cache-Control"))
		}
		if tc.status == http.StatusOK && (rw.Header().Get("Content-Type") != "application/json; charset=utf-8" || rw.Header().Get("X-Content-Type-Options") != "nosniff" || rw.Body.Len() == 0) {
			t.Errorf("%s: headers %v", tc.path, rw.Header())
		}
	}
}
