// Command tlsproxy is a TLS-terminating reverse proxy for the end-to-end
// tests. goned itself speaks plain HTTP and expects HTTPS to be terminated in
// front of it, as in production, so the tests put this proxy in front to give
// the browsers a real secure origin: the Secure cookie attribute, the
// secure-context checks and HTTPS-only behaviour are then exercised for real.
//
// Usage: tlsproxy listen=target [listen=target ...]
//
//	tlsproxy 127.0.0.1:8199=127.0.0.1:8299
//
// A throwaway self-signed certificate for 127.0.0.1, ::1 and localhost is
// generated in memory at startup and never written to disk. It is trusted by
// nothing, so Playwright runs with ignoreHTTPSErrors. This is test tooling
// only; nothing here ships with Gone.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// selfSigned returns a certificate valid for loopback names for one day.
func selfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("serial: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "gone e2e proxy"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// newProxy forwards every request to target. Responses are flushed as they
// arrive, so streamed claims behave as they do behind a real proxy, and the
// client's address is appended to X-Forwarded-For.
func newProxy(target string) *httputil.ReverseProxy {
	u := &url.URL{Scheme: "http", Host: target}
	p := httputil.NewSingleHostReverseProxy(u)
	p.FlushInterval = -1
	p.ErrorLog = log.New(io.Discard, "", 0)
	return p
}

// serve listens on addr with TLS and proxies to target until it fails.
func serve(addr, target string, cert tls.Certificate, errs chan<- error) {
	srv := &http.Server{
		Addr:              addr,
		Handler:           newProxy(target),
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs <- fmt.Errorf("%s: %w", addr, srv.ListenAndServeTLS("", ""))
}

// run parses the listen=target pairs and serves them all.
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: tlsproxy listen=target [listen=target ...]")
	}
	cert, err := selfSigned()
	if err != nil {
		return err
	}
	errs := make(chan error, len(args))
	for _, a := range args {
		listen, target, ok := strings.Cut(a, "=")
		if !ok || listen == "" || target == "" {
			return fmt.Errorf("bad mapping %q: want listen=target", a)
		}
		go serve(listen, target, cert, errs)
	}
	return <-errs
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
