package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const peer = "192.0.2.10:443" // an untrusted client

// providerCase is a signed provider with a token in its documented shape.
type providerCase struct {
	name     string
	s        Settings
	header   string
	prefix   string // e.g. "Bearer "
	alg      jose.SignatureAlgorithm
	issuer   string
	audience string
}

func signedProviders() []providerCase {
	return []providerCase{
		{"cloudflare", Settings{Provider: "cloudflare", Team: "acme", Audience: "4714c1358e65fe4b408ad6d432a5f878f08194bdb4752441fd56faefa9b2b6f2"},
			"Cf-Access-Jwt-Assertion", "", jose.RS256, "https://acme.cloudflareaccess.com", "4714c1358e65fe4b408ad6d432a5f878f08194bdb4752441fd56faefa9b2b6f2"},
		{"google-iap", Settings{Provider: "google-iap", Audience: "/projects/123/global/backendServices/456"},
			"X-Goog-Iap-Jwt-Assertion", "", jose.ES256, "https://cloud.google.com/iap", "/projects/123/global/backendServices/456"},
		{"pomerium", Settings{Provider: "pomerium", Host: "gone.example.com"},
			"X-Pomerium-Jwt-Assertion", "", jose.ES256, "gone.example.com", "gone.example.com"},
		{"teleport", Settings{Provider: "teleport", Host: "tp.example.com", Issuer: "example", Audience: "http://127.0.0.1:8080"},
			"Teleport-Jwt-Assertion", "", jose.RS256, "example", "http://127.0.0.1:8080"},
		{"oidc", Settings{Provider: "oidc", Issuer: "https://idp.example.com", Audience: "gone"},
			"Authorization", "Bearer ", jose.RS256, "https://idp.example.com", "gone"},
		{"oidc es256 vouch", Settings{Provider: "oidc", Issuer: "https://idp.example.com", Audience: "gone", Alg: "ES256", Header: "X-Vouch-IdP-IdToken"},
			"X-Vouch-IdP-IdToken", "", jose.ES256, "https://idp.example.com", "gone"},
	}
}

func TestSignedProviders(t *testing.T) {
	for _, pc := range signedProviders() {
		t.Run(pc.name, func(t *testing.T) {
			k := keyFor(t, pc.alg, "k1")
			ks := newKeyServer(t, k.jwk())
			g := signedGate(t, pc.s, ks)
			good := mint(t, k, validClaims(pc.issuer, pc.audience))
			if !g.SignedIn(req(peer, pc.header, pc.prefix+good)) {
				t.Fatal("a valid token was refused")
			}
			if g.SignedIn(req(peer, "X-Other", pc.prefix+good)) {
				t.Fatal("a token in the wrong header was accepted")
			}
			if pc.prefix == "" && g.SignedIn(req(peer, pc.header, "Bearer "+good)) != true {
				t.Fatal("a Bearer prefix should be tolerated")
			}
		})
	}
}

func TestSignedTokenRejections(t *testing.T) {
	pc := signedProviders()[0] // cloudflare, RS256
	k := keyFor(t, jose.RS256, "k1")
	ks := newKeyServer(t, k.jwk())
	g := signedGate(t, pc.s, ks)
	claims := func(edit func(*jwt.Claims)) string {
		c := validClaims(pc.issuer, pc.audience)
		edit(&c)
		return mint(t, k, c)
	}
	es := keyFor(t, jose.ES256, "k1")
	tests := map[string]string{
		"wrong issuer":       claims(func(c *jwt.Claims) { c.Issuer = "https://evil.cloudflareaccess.com" }),
		"wrong audience":     claims(func(c *jwt.Claims) { c.Audience = jwt.Audience{"other"} }),
		"expired":            claims(func(c *jwt.Claims) { c.Expiry = jwt.NewNumericDate(testNow.Add(-2 * time.Minute)) }),
		"not yet valid":      claims(func(c *jwt.Claims) { c.NotBefore = jwt.NewNumericDate(testNow.Add(2 * time.Minute)) }),
		"issued in future":   claims(func(c *jwt.Claims) { c.IssuedAt = jwt.NewNumericDate(testNow.Add(2 * time.Minute)) }),
		"no expiry":          claims(func(c *jwt.Claims) { c.Expiry = nil }),
		"no subject":         claims(func(c *jwt.Claims) { c.Subject = "" }),
		"other algorithm":    mint(t, es, validClaims(pc.issuer, pc.audience)),
		"hs256 with pub key": hs256(t, validClaims(pc.issuer, pc.audience)),
		"alg none":           algNone(t, validClaims(pc.issuer, pc.audience)),
		"tampered payload":   tamper(t, mint(t, k, validClaims(pc.issuer, pc.audience))),
		"garbage":            "not.a.jwt",
		"empty":              "",
		"oversized":          strings.Repeat("a", maxSignedHeader+1),
	}
	for name, tok := range tests {
		if g.SignedIn(req(peer, pc.header, tok)) {
			t.Errorf("%s: accepted", name)
		}
	}
	within := claims(func(c *jwt.Claims) { c.Expiry = jwt.NewNumericDate(testNow.Add(-30 * time.Second)) })
	if !g.SignedIn(req(peer, pc.header, within)) {
		t.Error("a token expired within the 60 s leeway was refused")
	}
}

func TestSignedHeaderRepeated(t *testing.T) {
	pc := signedProviders()[0]
	k := keyFor(t, jose.RS256, "k1")
	g := signedGate(t, pc.s, newKeyServer(t, k.jwk()))
	tok := mint(t, k, validClaims(pc.issuer, pc.audience))
	r := req(peer, pc.header, tok)
	r.Header.Add(pc.header, tok)
	if g.SignedIn(r) {
		t.Fatal("a repeated header was accepted")
	}
}

func TestKeyRotationAndRefetchLimit(t *testing.T) {
	pc := signedProviders()[0]
	old, rotated := keyFor(t, jose.RS256, "old"), keyFor(t, jose.RS256, "new")
	ks := newKeyServer(t, old.jwk())
	g := signedGate(t, pc.s, ks)
	clock := testNow.Add(10 * time.Minute)
	g.signed.keys.now = func() time.Time { return clock }
	g.signed.now = func() time.Time { return testNow }

	ks.set(rotated.jwk())
	tok := mint(t, rotated, validClaims(pc.issuer, pc.audience))
	if !g.SignedIn(req(peer, pc.header, tok)) {
		t.Fatal("a token from a rotated key wasn't accepted after a refetch")
	}
	hits := ks.hits.Load()
	unknown := mint(t, keyFor(t, jose.RS256, "forged"), validClaims(pc.issuer, pc.audience))
	for range 5 {
		if g.SignedIn(req(peer, pc.header, unknown)) {
			t.Fatal("a token with an unknown key ID was accepted")
		}
	}
	if got := ks.hits.Load(); got != hits {
		t.Fatalf("unknown key IDs refetched %d times within the limit", got-hits)
	}
}

func TestKeyIDMatching(t *testing.T) {
	pc := signedProviders()[0]
	k := keyFor(t, jose.RS256, "")
	g := signedGate(t, pc.s, newKeyServer(t, k.jwk()))
	if !g.SignedIn(req(peer, pc.header, mint(t, k, validClaims(pc.issuer, pc.audience)))) {
		t.Fatal("a token without a key ID was refused with a one-key set")
	}
	other := keyFor(t, jose.RS256, "b").jwk()
	other.KeyID = "b"
	g = signedGate(t, pc.s, newKeyServer(t, k.jwk(), other))
	if g.SignedIn(req(peer, pc.header, mint(t, k, validClaims(pc.issuer, pc.audience)))) {
		t.Fatal("a token without a key ID was accepted with several keys")
	}
	named := keyFor(t, jose.RS256, "k1")
	wrongAlg := named.jwk()
	wrongAlg.Algorithm = "RS512"
	g = signedGate(t, pc.s, newKeyServer(t, wrongAlg, keyFor(t, jose.ES256, "spare").jwk()))
	if g.SignedIn(req(peer, pc.header, mint(t, named, validClaims(pc.issuer, pc.audience)))) {
		t.Fatal("a key declaring another algorithm was used")
	}
}

func TestFailsClosedUntilKeysLoad(t *testing.T) {
	pc := signedProviders()[0]
	k := keyFor(t, jose.RS256, "k1")
	ks := newKeyServer(t, k.jwk())
	ks.code = 503
	g := New(pc.s, nil, ks.srv.Client())
	g.signed.now = func() time.Time { return testNow }
	g.signed.keys.url, g.signed.keys.retry = ks.srv.URL, 10*time.Millisecond
	tok := mint(t, k, validClaims(pc.issuer, pc.audience))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	if g.SignedIn(req(peer, pc.header, tok)) {
		t.Fatal("a token was accepted before any keys loaded")
	}
	ks.mu.Lock()
	ks.code = 200
	ks.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for !g.SignedIn(req(peer, pc.header, tok)) {
		if time.Now().After(deadline) {
			t.Fatal("keys never loaded after the server recovered")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPlainProviders(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	proxy := "10.0.0.2:5000"
	authelia := New(Settings{Provider: "authelia"}, trusted, nil)
	tests := []struct {
		name string
		g    *Gate
		peer string
		h, v string
		want bool
	}{
		{"authelia from proxy", authelia, proxy, "Remote-User", "alice", true},
		{"authelia from client", authelia, peer, "Remote-User", "alice", false},
		{"authelia empty", authelia, proxy, "Remote-User", "  ", false},
		{"authelia missing", authelia, proxy, "", "", false},
		{"authelia oversized", authelia, proxy, "Remote-User", strings.Repeat("a", maxPlainHeader+1), false},
		{"authentik", New(Settings{Provider: "authentik"}, trusted, nil), proxy, "X-Authentik-Username", "alice", true},
		{"tailscale", New(Settings{Provider: "tailscale"}, trusted, nil), proxy, "Tailscale-User-Login", "alice@example.com", true},
		{"custom any", New(Settings{Provider: Custom, Header: "X-Gone-User"}, trusted, nil), proxy, "X-Gone-User", "bob", true},
		{"custom email ok", New(Settings{Provider: Custom, Header: "X-Email", Format: "email"}, trusted, nil), proxy, "X-Email", "bob@example.com", true},
		{"custom email name", New(Settings{Provider: Custom, Header: "X-Email", Format: "email"}, trusted, nil), proxy, "X-Email", "Bob <bob@example.com>", false},
		{"custom email bad", New(Settings{Provider: Custom, Header: "X-Email", Format: "email"}, trusted, nil), proxy, "X-Email", "bob", false},
		{"custom exact ok", New(Settings{Provider: Custom, Header: "X-Proxy-Key", Format: "exact", Value: "s3cret-value-123456"}, trusted, nil), proxy, "X-Proxy-Key", "s3cret-value-123456", true},
		{"custom exact bad", New(Settings{Provider: Custom, Header: "X-Proxy-Key", Format: "exact", Value: "s3cret-value-123456"}, trusted, nil), proxy, "X-Proxy-Key", "s3cret-value-12345", false},
	}
	for _, tc := range tests {
		if got := tc.g.SignedIn(req(tc.peer, tc.h, tc.v)); got != tc.want {
			t.Errorf("%s: SignedIn = %v, want %v", tc.name, got, tc.want)
		}
	}
	r := req(proxy, "Remote-User", "alice")
	r.Header.Add("Remote-User", "mallory")
	if authelia.SignedIn(r) {
		t.Error("a repeated plain header was accepted")
	}
}

func TestOffAndSignInURL(t *testing.T) {
	if New(Settings{}, nil, nil) != nil || New(Settings{Provider: None}, nil, nil) != nil {
		t.Fatal("sign-in off must give a nil gate")
	}
	var off *Gate
	off.Start(context.Background())
	if off.SignInURL("/") != "" {
		t.Fatal("nil gate has a sign-in URL")
	}
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	g := New(Settings{Provider: "authelia", SignInURL: "/oauth2/start?rd={return}"}, trusted, nil)
	g.Start(context.Background()) // nothing to load for a header provider
	if got := g.SignInURL("/request"); got != "/oauth2/start?rd=%2Frequest" {
		t.Fatalf("SignInURL = %q", got)
	}
	if got := New(Settings{Provider: "authelia"}, trusted, nil).SignInURL("/"); got != "" {
		t.Fatalf("no URL configured, got %q", got)
	}
}

func TestStripBearer(t *testing.T) {
	for in, want := range map[string]string{"Bearer abc": "abc", "bearer  abc": "abc", "BEARER x": "x", "abc": "abc", "Bearer": "Bearer", "Bearer ": "Bearer "} {
		if got := stripBearer(in); got != want {
			t.Errorf("stripBearer(%q) = %q, want %q", in, got, want)
		}
	}
}

// hs256 signs claims with HMAC-SHA256, keyed with the RSA public key's
// modulus, the classic algorithm-confusion forgery.
func hs256(t *testing.T, claims jwt.Claims) string {
	t.Helper()
	keyFor(t, jose.RS256, "k1") // make sure rsaPriv exists
	secret := []byte(base64.RawURLEncoding.EncodeToString(rsaPriv.N.Bytes()))
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","kid":"k1","typ":"JWT"}`))
	body := payload(t, claims)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(head + "." + body))
	return head + "." + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// algNone builds an unsigned token.
func algNone(t *testing.T, claims jwt.Claims) string {
	t.Helper()
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"k1"}`))
	return head + "." + payload(t, claims) + "."
}

// tamper swaps a valid token's payload for one with another subject.
func tamper(t *testing.T, tok string) string {
	t.Helper()
	parts := strings.Split(tok, ".")
	c := validClaims("https://acme.cloudflareaccess.com", "x")
	c.Subject = "admin"
	parts[1] = payload(t, c)
	return strings.Join(parts, ".")
}

// payload encodes claims as a JWT payload segment.
func payload(t *testing.T, claims jwt.Claims) string {
	t.Helper()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// TestRefetchSurvivesHangUp checks a client that hangs up can't cancel the
// refetch for an unknown key ID, which would keep rotated keys unloaded.
func TestRefetchSurvivesHangUp(t *testing.T) {
	pc := signedProviders()[0]
	old, rotated := keyFor(t, jose.RS256, "old"), keyFor(t, jose.RS256, "new")
	ks := newKeyServer(t, old.jwk())
	g := signedGate(t, pc.s, ks)
	g.signed.keys.now = func() time.Time { return testNow.Add(10 * time.Minute) }
	ks.set(rotated.jwk())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the forger already hung up
	forged := req(peer, pc.header, mint(t, keyFor(t, jose.RS256, "forged"), validClaims(pc.issuer, pc.audience)))
	if g.SignedIn(forged.WithContext(ctx)) {
		t.Fatal("a forged key ID was accepted")
	}
	tok := mint(t, rotated, validClaims(pc.issuer, pc.audience))
	if !g.SignedIn(req(peer, pc.header, tok)) {
		t.Fatal("the rotated key wasn't loaded by the refetch the forged token triggered")
	}
}
