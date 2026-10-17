package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// TestGoogleKeySetParses reads Google IAP's real public key set.
func TestGoogleKeySetParses(t *testing.T) {
	raw, err := os.ReadFile("testdata/google-iap-public_key-jwk.json")
	if err != nil {
		t.Fatal(err)
	}
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	keys := publicKeys(set.Keys)
	if len(keys) == 0 || len(keys) != len(set.Keys) {
		t.Fatalf("parsed %d of %d Google keys", len(keys), len(set.Keys))
	}
	for _, k := range keys {
		if k.Algorithm != "ES256" || k.KeyID == "" {
			t.Errorf("unexpected key %s %s", k.KeyID, k.Algorithm)
		}
	}
}

// TestCloudflareCertsShape reads a response shaped like Cloudflare Access's
// /cdn-cgi/access/certs: keys plus PEM certificates, which are ignored.
func TestCloudflareCertsShape(t *testing.T) {
	k := keyFor(t, jose.RS256, "cf1")
	pub := rsaPriv.PublicKey
	body := map[string]any{
		"keys": []map[string]string{{
			"kid": "cf1", "kty": "RSA", "alg": "RS256", "use": "sig",
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		}},
		"public_cert":  map[string]string{"kid": "cf1", "cert": "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n"},
		"public_certs": []map[string]string{{"kid": "cf1", "cert": "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n"}},
	}
	ks := newKeyServer(t)
	ks.body, _ = json.Marshal(body)
	pc := signedProviders()[0]
	g := signedGate(t, pc.s, ks)
	if !g.SignedIn(req(peer, pc.header, mint(t, k, validClaims(pc.issuer, pc.audience)))) {
		t.Fatal("a token signed by the Cloudflare-shaped key set was refused")
	}
}

func TestPublicKeysSkipsUnusable(t *testing.T) {
	pub, _ := json.Marshal(keyFor(t, jose.RS256, "ok").jwk())
	encUse := keyFor(t, jose.ES256, "enc").jwk()
	encUse.Use = "enc"
	enc, _ := json.Marshal(encUse)
	private, _ := json.Marshal(jose.JSONWebKey{Key: rsaPriv, KeyID: "priv", Algorithm: "RS256"})
	keys := publicKeys([]json.RawMessage{pub, enc, private, json.RawMessage(`{"kty":"weird"}`), json.RawMessage(`[`)})
	if len(keys) != 1 || keys[0].KeyID != "ok" {
		t.Fatalf("kept %d keys: %+v", len(keys), keys)
	}
}

func TestFetchFailures(t *testing.T) {
	tests := map[string]func(*keyServer){
		"status":   func(ks *keyServer) { ks.code = http.StatusNotFound },
		"bad json": func(ks *keyServer) { ks.body = []byte("{") },
		"no keys":  func(ks *keyServer) { ks.body = []byte(`{"keys":[]}`) },
		"too big":  func(ks *keyServer) { ks.body = []byte(`{"keys":[],"pad":"` + strings.Repeat("a", maxKeysBody) + `"}`) },
	}
	for name, breakIt := range tests {
		ks := newKeyServer(t)
		breakIt(ks)
		src := newKeySource(endpoints{keysURL: ks.srv.URL}, ks.srv.Client())
		if err := src.load(context.Background()); err == nil {
			t.Errorf("%s: load succeeded", name)
		}
	}
	plain := httptest.NewServer(http.NotFoundHandler())
	defer plain.Close()
	if err := newKeySource(endpoints{keysURL: plain.URL}, nil).load(context.Background()); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("plain-http key URL: %v", err)
	}
}

func TestDiscovery(t *testing.T) {
	k := keyFor(t, jose.RS256, "k1")
	ks := newKeyServer(t, k.jwk())
	var issuer, keysURI string
	disc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": keysURI})
	}))
	defer disc.Close()
	issuer, keysURI = disc.URL, ks.srv.URL+"/keys"
	src := newKeySource(endpoints{keysURL: disc.URL + "/.well-known/openid-configuration", issuer: disc.URL, discover: true}, disc.Client())
	if err := src.load(context.Background()); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if src.keysURL != ks.srv.URL+"/keys" || len(src.keys) != 1 {
		t.Fatalf("keys from %q: %d", src.keysURL, len(src.keys))
	}
	issuer = "https://someone-else.example"
	if err := src.load(context.Background()); err == nil || !strings.Contains(err.Error(), "does not match GONE_AUTH_ISSUER") {
		t.Fatalf("issuer mismatch: %v", err)
	}
	if len(src.keys) != 1 {
		t.Fatal("a failed refresh dropped the last good keys")
	}
	issuer, keysURI = disc.URL, "http://plain.example/keys"
	if err := src.load(context.Background()); err == nil || !strings.Contains(err.Error(), "not an https URL") {
		t.Fatalf("plain jwks_uri: %v", err)
	}
}

func TestHTTPSOnlyRedirects(t *testing.T) {
	to := func(raw string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		return r
	}
	if httpsOnly(to("http://example.com/keys"), nil) == nil {
		t.Error("a redirect to plain http was allowed")
	}
	if httpsOnly(to("https://example.com/keys"), make([]*http.Request, 5)) == nil {
		t.Error("a sixth redirect was allowed")
	}
	if err := httpsOnly(to("https://example.com/keys"), nil); err != nil {
		t.Errorf("an https redirect was refused: %v", err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ks := newKeyServer(t, keyFor(t, jose.RS256, "k1").jwk())
	src := newKeySource(endpoints{keysURL: ks.srv.URL}, ks.srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for src.find("k1", jose.RS256) == nil {
		if time.Now().After(deadline) {
			t.Fatal("keys never loaded")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	failing := newKeySource(endpoints{keysURL: "https://127.0.0.1:1/keys"}, nil)
	failing.retry = 0
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	failing.run(ctx2) // returns at once: the first load fails and the context is done
}
