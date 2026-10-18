package auth

import (
	"slices"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		s       Settings
		trusted bool
		wantErr string
	}{
		{name: "off", s: Settings{}},
		{name: "none", s: Settings{Provider: None}},
		{name: "leftover option", s: Settings{Team: "acme"}, wantErr: "GONE_AUTH_TEAM: set without GONE_AUTH"},
		{name: "leftover sign-in url", s: Settings{SignInURL: "/login"}, wantErr: "GONE_AUTH_SIGNIN_URL: set without GONE_AUTH"},
		{name: "option with none", s: Settings{Provider: None, Team: "acme"}, wantErr: "GONE_AUTH_TEAM: set with GONE_AUTH=none"},
		{name: "sign-in url with none", s: Settings{Provider: None, SignInURL: "/login"}, wantErr: "GONE_AUTH_SIGNIN_URL: set with GONE_AUTH=none"},
		{name: "unknown", s: Settings{Provider: "okta"}, wantErr: `GONE_AUTH: unknown provider "okta"`},
		{name: "cloudflare", s: Settings{Provider: "cloudflare", Team: "acme", Audience: "tag"}},
		{name: "cloudflare team domain", s: Settings{Provider: "cloudflare", Team: "acme.cloudflareaccess.com", Audience: "tag"}},
		{name: "cloudflare missing audience", s: Settings{Provider: "cloudflare", Team: "acme"}, wantErr: "GONE_AUTH_AUDIENCE: required with GONE_AUTH=cloudflare"},
		{name: "cloudflare bad team", s: Settings{Provider: "cloudflare", Team: "https://acme", Audience: "a"}, wantErr: "GONE_AUTH_TEAM: must be a host name"},
		{name: "google extra option", s: Settings{Provider: "google-iap", Audience: "/projects/1/global/backendServices/2", Header: "X-User"}, wantErr: "GONE_AUTH_HEADER: does not apply to GONE_AUTH=google-iap"},
		{name: "pomerium", s: Settings{Provider: "pomerium", Host: "gone.example.com"}},
		{name: "pomerium keys url", s: Settings{Provider: "pomerium", Host: "gone.example.com", KeysURL: "https://pomerium.internal:8443/.well-known/pomerium/jwks.json"}},
		{name: "pomerium http keys", s: Settings{Provider: "pomerium", Host: "gone.example.com", KeysURL: "http://pomerium/jwks.json"}, wantErr: "GONE_AUTH_JWKS_URL: must be an https:// URL"},
		{name: "pomerium host with scheme", s: Settings{Provider: "pomerium", Host: "https://gone.example.com"}, wantErr: "GONE_AUTH_HOST: must be a host name"},
		{name: "teleport", s: Settings{Provider: "teleport", Host: "teleport.example.com:443", Issuer: "example", Audience: "http://127.0.0.1:8080"}},
		{name: "teleport missing issuer", s: Settings{Provider: "teleport", Host: "teleport.example.com", Audience: "x"}, wantErr: "GONE_AUTH_ISSUER: required"},
		{name: "oidc", s: Settings{Provider: "oidc", Issuer: "https://idp.example.com/realms/acme", Audience: "gone"}},
		{name: "oidc es256 header", s: Settings{Provider: "oidc", Issuer: "https://idp.example.com", Audience: "gone", Alg: "ES256", Header: "X-Vouch-IdP-IdToken"}},
		{name: "oidc http issuer", s: Settings{Provider: "oidc", Issuer: "http://idp.example.com", Audience: "gone"}, wantErr: "GONE_AUTH_ISSUER: must be an https:// URL"},
		{name: "oidc bad alg", s: Settings{Provider: "oidc", Issuer: "https://idp.example.com", Audience: "gone", Alg: "HS256"}, wantErr: "GONE_AUTH_ALG: use one of RS256, ES256"},
		{name: "authelia", s: Settings{Provider: "authelia"}, trusted: true},
		{name: "authelia untrusted", s: Settings{Provider: "authelia"}, wantErr: "GONE_TRUSTED_PROXIES: required with GONE_AUTH=authelia"},
		{name: "tailscale option", s: Settings{Provider: "tailscale", Header: "X-User"}, trusted: true, wantErr: "GONE_AUTH_HEADER: does not apply"},
		{name: "custom", s: Settings{Provider: Custom, Header: "X-Gone-User", Format: "email"}, trusted: true},
		{name: "custom no header", s: Settings{Provider: Custom}, trusted: true, wantErr: "GONE_AUTH_HEADER: required with GONE_AUTH=custom"},
		{name: "custom underscore", s: Settings{Provider: Custom, Header: "X_Gone_User"}, trusted: true, wantErr: "may use only letters, digits and dashes"},
		{name: "custom long header", s: Settings{Provider: Custom, Header: strings.Repeat("a", headerMax+1)}, trusted: true, wantErr: "at most 64 characters"},
		{name: "custom bad format", s: Settings{Provider: Custom, Header: "X-U", Format: "regex"}, trusted: true, wantErr: "GONE_AUTH_FORMAT: use one of any, email, exact"},
		{name: "custom exact no value", s: Settings{Provider: Custom, Header: "X-U", Format: "exact"}, trusted: true, wantErr: "GONE_AUTH_VALUE: required with GONE_AUTH_FORMAT=exact"},
		{name: "custom value without exact", s: Settings{Provider: Custom, Header: "X-U", Value: "abc"}, trusted: true, wantErr: "GONE_AUTH_VALUE: applies only with GONE_AUTH_FORMAT=exact"},
		{name: "custom untrusted", s: Settings{Provider: Custom, Header: "X-U"}, wantErr: "GONE_TRUSTED_PROXIES: required"},
		{name: "sign-in path", s: Settings{Provider: "authelia", SignInURL: "/oauth2/start?rd={return}"}, trusted: true},
		{name: "sign-in https", s: Settings{Provider: "authelia", SignInURL: "https://auth.example.com/?rd=https://gone.example.com{return}"}, trusted: true},
		{name: "sign-in protocol-relative", s: Settings{Provider: "authelia", SignInURL: "//evil.example"}, trusted: true, wantErr: "GONE_AUTH_SIGNIN_URL"},
		{name: "sign-in backslash", s: Settings{Provider: "authelia", SignInURL: `/\evil.example`}, trusted: true, wantErr: "GONE_AUTH_SIGNIN_URL"},
		{name: "sign-in http", s: Settings{Provider: "authelia", SignInURL: "http://auth.example.com"}, trusted: true, wantErr: "GONE_AUTH_SIGNIN_URL"},
		{name: "sign-in javascript", s: Settings{Provider: "authelia", SignInURL: "javascript:alert(1)"}, trusted: true, wantErr: "GONE_AUTH_SIGNIN_URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.s.Validate(tc.trusted)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateWarnings(t *testing.T) {
	w, err := Settings{Provider: "google-iap", Audience: "/projects/1/apps/2"}.Validate(false)
	if err != nil || len(w) != 1 || !strings.Contains(w[0], "GONE_AUTH=google-iap without GONE_TRUSTED_PROXIES") {
		t.Fatalf("signed without proxies: %v, %v", w, err)
	}
	if w, _ = (Settings{Provider: "google-iap", Audience: "a"}).Validate(true); len(w) != 0 {
		t.Fatalf("signed with proxies warned: %v", w)
	}
	w, err = Settings{Provider: Custom, Header: "X-Proxy-Key", Format: "exact", Value: "zq7x"}.Validate(true)
	if err != nil || len(w) != 1 || !strings.Contains(w[0], "GONE_AUTH_VALUE is shorter than 16") {
		t.Fatalf("short exact value: %v, %v", w, err)
	}
	if strings.Contains(w[0], "zq7x") {
		t.Fatal("the warning must not contain the value")
	}
}

func TestProviders(t *testing.T) {
	got := Providers()
	for _, want := range []string{None, Custom, "cloudflare", "google-iap", "pomerium", "teleport", "oidc", "authelia", "authentik", "tailscale"} {
		if !slices.Contains(got, want) {
			t.Errorf("Providers() lacks %q", want)
		}
	}
	if !slices.IsSorted(got) {
		t.Errorf("Providers() not sorted: %v", got)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		s    Settings
		want endpoints
	}{
		{Settings{Provider: "cloudflare", Team: "acme", Audience: "tag"},
			endpoints{keysURL: "https://acme.cloudflareaccess.com/cdn-cgi/access/certs", issuer: "https://acme.cloudflareaccess.com", audience: "tag"}},
		{Settings{Provider: "cloudflare", Team: "acme.cloudflareaccess.com", Audience: "tag"},
			endpoints{keysURL: "https://acme.cloudflareaccess.com/cdn-cgi/access/certs", issuer: "https://acme.cloudflareaccess.com", audience: "tag"}},
		{Settings{Provider: "google-iap", Audience: "/projects/1/global/backendServices/2"},
			endpoints{keysURL: "https://www.gstatic.com/iap/verify/public_key-jwk", issuer: "https://cloud.google.com/iap", audience: "/projects/1/global/backendServices/2"}},
		{Settings{Provider: "pomerium", Host: "gone.example.com"},
			endpoints{keysURL: "https://gone.example.com/.well-known/pomerium/jwks.json", issuer: "gone.example.com", audience: "gone.example.com"}},
		{Settings{Provider: "pomerium", Host: "gone.example.com", KeysURL: "https://p.internal/jwks.json"},
			endpoints{keysURL: "https://p.internal/jwks.json", issuer: "gone.example.com", audience: "gone.example.com"}},
		{Settings{Provider: "teleport", Host: "tp.example.com", Issuer: "example", Audience: "http://127.0.0.1:8080"},
			endpoints{keysURL: "https://tp.example.com/.well-known/jwks.json", issuer: "example", audience: "http://127.0.0.1:8080"}},
		{Settings{Provider: "oidc", Issuer: "https://idp.example.com/realms/acme/", Audience: "gone"},
			endpoints{keysURL: "https://idp.example.com/realms/acme/.well-known/openid-configuration", issuer: "https://idp.example.com/realms/acme/", audience: "gone", discover: true}},
		{Settings{Provider: "oidc", Issuer: "https://idp.example.com", Audience: "gone", KeysURL: "https://idp.internal/keys"},
			endpoints{keysURL: "https://idp.internal/keys", issuer: "https://idp.example.com", audience: "gone"}},
	}
	for _, tc := range tests {
		if got := specs[tc.s.Provider].resolve(tc.s); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.s.Provider, got, tc.want)
		}
	}
}

func TestPlainHeader(t *testing.T) {
	for _, p := range []string{"authelia", "authentik", "tailscale", Custom} {
		if !(Settings{Provider: p}).PlainHeader() {
			t.Errorf("%s: want plain header", p)
		}
	}
	for _, p := range []string{"", None, "cloudflare", "google-iap", "pomerium", "teleport", "oidc", "okta"} {
		if (Settings{Provider: p}).PlainHeader() {
			t.Errorf("%q: not a plain-header provider", p)
		}
	}
}
