package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// testNow is the fixed clock the gate tests use.
var testNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// signingKey is a private key with its key ID and algorithm.
type signingKey struct {
	kid  string
	alg  jose.SignatureAlgorithm
	priv any
	pub  any
}

var (
	keyOnce sync.Once
	rsaPriv *rsa.PrivateKey
	ecPriv  *ecdsa.PrivateKey
)

// keyFor returns an RSA-2048 (RS256) or P-256 (ES256) key, generated once.
func keyFor(t *testing.T, alg jose.SignatureAlgorithm, kid string) signingKey {
	t.Helper()
	keyOnce.Do(func() {
		var err error
		if rsaPriv, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
		if ecPriv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			panic(err)
		}
	})
	if alg == jose.ES256 {
		return signingKey{kid: kid, alg: alg, priv: ecPriv, pub: &ecPriv.PublicKey}
	}
	return signingKey{kid: kid, alg: alg, priv: rsaPriv, pub: &rsaPriv.PublicKey}
}

// jwk returns the public key as a key-set member.
func (k signingKey) jwk() jose.JSONWebKey {
	return jose.JSONWebKey{Key: k.pub, KeyID: k.kid, Algorithm: string(k.alg), Use: "sig"}
}

// mint signs claims (any JSON-able value) as a compact JWT.
func mint(t *testing.T, k signingKey, claims any) string {
	t.Helper()
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: k.alg, Key: jose.JSONWebKey{Key: k.priv, KeyID: k.kid}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jwt.Signed(sig).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// validClaims are claims valid at testNow for issuer and audience.
func validClaims(issuer, audience string) jwt.Claims {
	return jwt.Claims{
		Issuer: issuer, Subject: "user-123", Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(testNow.Add(-time.Minute)), NotBefore: jwt.NewNumericDate(testNow.Add(-time.Minute)),
		Expiry: jwt.NewNumericDate(testNow.Add(5 * time.Minute)),
	}
}

// keyServer serves a key set over TLS and counts requests.
type keyServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	keys []jose.JSONWebKey
	body []byte // when set, served instead of the key set
	code int
	hits atomic.Int32
}

func newKeyServer(t *testing.T, keys ...jose.JSONWebKey) *keyServer {
	t.Helper()
	ks := &keyServer{keys: keys, code: http.StatusOK}
	ks.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ks.hits.Add(1)
		ks.mu.Lock()
		defer ks.mu.Unlock()
		w.WriteHeader(ks.code)
		if ks.body != nil {
			_, _ = w.Write(ks.body)
			return
		}
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: ks.keys})
	}))
	t.Cleanup(ks.srv.Close)
	return ks
}

// set replaces the served keys.
func (ks *keyServer) set(keys ...jose.JSONWebKey) {
	ks.mu.Lock()
	ks.keys = keys
	ks.mu.Unlock()
}

// signedGate builds the gate for s with its keys pointed at ks, the clock
// at testNow, and the keys loaded.
func signedGate(t *testing.T, s Settings, ks *keyServer) *Gate {
	t.Helper()
	if _, err := s.Validate(true); err != nil {
		t.Fatalf("settings: %v", err)
	}
	g := New(s, nil, ks.srv.Client())
	g.signed.now = func() time.Time { return testNow }
	g.signed.keys.url, g.signed.keys.discover = ks.srv.URL+"/keys", false
	g.signed.keys.now = func() time.Time { return testNow }
	if err := g.signed.keys.load(context.Background()); err != nil {
		t.Fatalf("load keys: %v", err)
	}
	return g
}

// req builds a request from peer with one header set to value.
func req(peer, header, value string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/secret", nil)
	r.RemoteAddr = peer
	if header != "" {
		r.Header.Set(header, value)
	}
	return r
}
