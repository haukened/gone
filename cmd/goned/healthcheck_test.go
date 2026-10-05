package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeURL(t *testing.T) {
	cases := map[string]string{
		":8080":           "http://127.0.0.1:8080/readyz",
		"0.0.0.0:8080":    "http://127.0.0.1:8080/readyz",
		"[::]:8080":       "http://[::1]:8080/readyz",
		"127.0.0.1:9000":  "http://127.0.0.1:9000/readyz",
		"[::1]:8080":      "http://[::1]:8080/readyz",
		"192.0.2.10:8080": "http://192.0.2.10:8080/readyz",
	}
	for addr, want := range cases {
		got, err := probeURL(addr)
		if err != nil || got != want {
			t.Errorf("probeURL(%q) = %q, %v; want %q", addr, got, err, want)
		}
	}
	if _, err := probeURL("8080"); err == nil {
		t.Error("probeURL without a port: want an error")
	}
}

func TestProbe(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	ctx := context.Background()
	if err := probe(ctx, srv.Client(), srv.URL+"/readyz"); err != nil {
		t.Fatalf("ready server: %v", err)
	}
	status = http.StatusServiceUnavailable
	if err := probe(ctx, srv.Client(), srv.URL+"/readyz"); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("not-ready server: got %v, want a 503 error", err)
	}
	if err := probe(ctx, srv.Client(), "http://[::1"); err == nil {
		t.Fatal("bad URL: want an error")
	}
}

func TestHealthcheckExitCodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Setenv("GONE_ADDR", strings.TrimPrefix(srv.URL, "http://"))
	var stderr bytes.Buffer
	if code := healthcheck(&stderr); code != 0 {
		t.Fatalf("ready: exit %d, stderr %q", code, stderr.String())
	}

	srv.Close()
	if code := healthcheck(&stderr); code != 1 || !strings.Contains(stderr.String(), "goned healthcheck:") {
		t.Fatalf("down: exit %d, stderr %q", code, stderr.String())
	}

	stderr.Reset()
	t.Setenv("GONE_ADDR", "localhost:8080")
	if code := healthcheck(&stderr); code != 1 || !strings.Contains(stderr.String(), "GONE_ADDR") {
		t.Fatalf("bad config: exit %d, stderr %q", code, stderr.String())
	}
}
