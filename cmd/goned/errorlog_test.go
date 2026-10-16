package main

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/config"
)

// TestRedactAddrs covers IPv4 and IPv6 socket addresses inside messages.
func TestRedactAddrs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http: panic serving 192.0.2.7:51234: boom", "http: panic serving <addr>: boom"},
		{"http: panic serving [2001:db8::1]:443: boom", "http: panic serving <addr>: boom"},
		{"read tcp 10.0.0.1:80->203.0.113.9:5555: reset", "read tcp <addr>-><addr>: reset"},
		{"read tcp [::1]:80->[fe80::1%eth0]:5: reset", "read tcp <addr>-><addr>: reset"},
		{"no address here, version 1.2.3", "no address here, version 1.2.3"},
	}
	for _, c := range cases {
		if got := redactAddrs(c.in); got != c.want {
			t.Errorf("redactAddrs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// serveAndPanic serves a panicking handler on loopback with srv's settings and
// issues one request, returning once the connection has been dropped.
func serveAndPanic(t *testing.T, srv *http.Server) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	if resp, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
		_ = resp.Body.Close()
	}
}

// waitLog polls buf until it contains want or the deadline passes.
func waitLog(t *testing.T, buf *syncBuffer, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := buf.String(); strings.Contains(s, want) {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log never contained %q: %s", want, buf.String())
	return ""
}

// TestServers_PanicLogHasNoAddress checks both listeners log handler panics at
// warn level without the client address.
func TestServers_PanicLogHasNoAddress(t *testing.T) {
	servers := map[string]*http.Server{
		"public":  newServer(&config.Config{Addr: "127.0.0.1:0"}, nil),
		"metrics": newMetricsServer("127.0.0.1:0", nil),
	}
	for name, srv := range servers {
		t.Run(name, func(t *testing.T) {
			buf := captureLogs(t)
			serveAndPanic(t, srv)
			out := waitLog(t, buf, "panic serving")
			if strings.Contains(out, "127.0.0.1") {
				t.Fatalf("log leaked the client address: %s", out)
			}
			if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "<addr>") {
				t.Fatalf("want a warn line with the address redacted: %s", out)
			}
		})
	}
}
