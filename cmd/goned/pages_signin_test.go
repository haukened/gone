package main

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/config"
)

// signInConfig turns on custom sign-in trusting X-Gone-User from
// httptest's default peer, 192.0.2.1, with an optional sign-in URL.
func signInConfig(signInURL string) func(*config.Config) {
	return func(c *config.Config) {
		c.Auth, c.AuthHeader, c.AuthSignInURL = "custom", "X-Gone-User", signInURL
		c.TrustedPrefixes = []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")}
	}
}

// TestPages_SignInView checks the send and request pages swap their forms
// for the sign-in view, with the button only when a URL is set.
func TestPages_SignInView(t *testing.T) {
	withURL := pageRouterWith(t, signInConfig("/oauth2/start?rd={return}"))
	noURL := pageRouterWith(t, signInConfig(""))
	cases := []struct {
		h       http.Handler
		path    string
		want    []string
		notWant []string
	}{
		{withURL, "/", []string{`id="sign-in"`, `href="/oauth2/start?rd=%2f"`, `data-i18n="common.signInButton"`}, []string{`id="create-secret"`}},
		{withURL, "/request", []string{`id="sign-in"`, `href="/oauth2/start?rd=%2frequest"`}, []string{`id="create-request"`, `id="request-list"`}},
		{noURL, "/", []string{`id="sign-in"`, `data-i18n="common.signInText"`}, []string{`id="sign-in-link"`, `id="create-secret"`}},
		{withURL, "/reply/" + strings.Repeat("d", 32), []string{`id="create-secret"`}, []string{`id="sign-in"`}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			code, body := get(tc.h, tc.path)
			if code != http.StatusOK {
				t.Fatalf("status %d", code)
			}
			for _, s := range tc.want {
				if !strings.Contains(strings.ToLower(body), strings.ToLower(s)) {
					t.Errorf("missing %s", s)
				}
			}
			for _, s := range tc.notWant {
				if strings.Contains(body, s) {
					t.Errorf("unexpected %s", s)
				}
			}
			ids := idSet(t, body)
			requireDeclaredRefs(t, ids, body)
			requireNoInlineCode(t, body)
		})
	}
}

// TestPages_SignedInSeesForm checks a signed-in visitor gets the forms.
func TestPages_SignedInSeesForm(t *testing.T) {
	h := pageRouterWith(t, signInConfig("/login"))
	for path, form := range map[string]string{"/": `id="create-secret"`, "/request": `id="create-request"`} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("X-Gone-User", "alice")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if !strings.Contains(w.Body.String(), form) || strings.Contains(w.Body.String(), `id="sign-in"`) {
			t.Errorf("%s: signed-in visitor did not get the form", path)
		}
	}
}
