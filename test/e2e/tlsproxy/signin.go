package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

// Sign-in stand-ins for the GONE_AUTH end-to-end tests. A backend mapped
// with "+custom" or "+oidc" gets an identity only when the browser carries
// the test cookie, which the stub login page at /e2e-login sets. Like a real
// sign-in proxy, the proxy always removes identity headers the browser sent.
const (
	signInCookie = "gone_e2e_signin"
	e2eAudience  = "gone-e2e"
	customUser   = "e2e@example.com"
)

// idp is a minimal OpenID issuer: a discovery document, a key set and
// short-lived RS256 ID tokens for the "+oidc" backends.
type idp struct {
	issuer string
	key    *rsa.PrivateKey
}

func newIDP(addr string) (*idp, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate idp key: %w", err)
	}
	return &idp{issuer: "https://" + addr, key: key}, nil
}

// ServeHTTP serves discovery and the key set.
func (p *idp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": p.issuer, "jwks_uri": p.issuer + "/keys"})
	case "/keys":
		pub := p.key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "e2e", "alg": "RS256", "use": "sig",
			"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	default:
		http.NotFound(w, r)
	}
}

// mint returns a five-minute RS256 ID token for the e2e user.
func (p *idp) mint() string {
	now := time.Now()
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "e2e", "typ": "JWT"})
	body, _ := json.Marshal(map[string]any{
		"iss": p.issuer, "aud": e2eAudience, "sub": "e2e-user", "email": customUser,
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	signing := b64(head) + "." + b64(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return signing + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// withSignIn wraps a backend's proxy with the sign-in stand-in for mode
// ("custom" or "oidc"); an empty mode leaves the proxy as it is.
func withSignIn(next http.Handler, mode string, p *idp) http.Handler {
	if mode == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/e2e-login" {
			login(w, r)
			return
		}
		r.Header.Del("X-Gone-User")
		r.Header.Del("Authorization")
		if c, err := r.Cookie(signInCookie); err == nil && c.Value == "1" {
			if mode == "custom" {
				r.Header.Set("X-Gone-User", customUser)
			} else {
				r.Header.Set("Authorization", "Bearer "+p.mint())
			}
		}
		next.ServeHTTP(w, r)
	})
}

// login is the stub sign-in page: it sets the test cookie and sends the
// browser back to rd, a same-origin path (no "//" and no backslash, which
// browsers read as another host).
func login(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: signInCookie, Value: "1", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	rd := r.URL.Query().Get("rd")
	if !strings.HasPrefix(rd, "/") || strings.HasPrefix(rd, "//") || strings.Contains(rd, `\`) {
		rd = "/"
	}
	http.Redirect(w, r, rd, http.StatusFound)
}

// writeCert writes the proxy's certificate as PEM, so goned can trust the
// stand-in issuer through SSL_CERT_FILE.
func writeCert(path string, der []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
}
