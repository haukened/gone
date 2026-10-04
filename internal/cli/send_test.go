package cli

import (
	"net/http"
	"strings"
	"testing"
)

// TestSendErrors checks send failures, none of which may store a secret.
//
// Parameters:
//   - t: the test.
func TestSendErrors(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	tests := []struct {
		name  string
		args  []string
		stdin string
		code  int
		want  string
	}{
		{"positional", []string{"send", "--server", srv.URL, "msg"}, "", exitUsage, "no arguments"},
		{"zero ttl", []string{"send", "--server", srv.URL, "--ttl", "0s"}, "x", exitUsage, "--ttl"},
		{"bad flag", []string{"send", "--nope"}, "x", exitUsage, "nope"},
		{"bad server", []string{"send", "--server", "ftp://x"}, "x", exitUsage, "server"},
		{"http server", []string{"send", "--server", "http://example.com"}, "x", exitUsage, "--insecure"},
		{"empty", []string{"send", "--server", srv.URL}, "", exitUsage, "empty"},
		{"short passphrase", []string{"send", "--server", srv.URL, "--passphrase-file", writeTemp(t, "p", "short")}, "x", exitUsage, "8"},
		{"ttl too long", []string{"send", "--server", srv.URL, "--ttl", "10000h"}, "x", exitNetwork, "rejected"},
		{"missing file", []string{"send", "--server", srv.URL, "--file", "/nonexistent/gone-test"}, "x", exitIO, "attachment"},
	}
	for _, tt := range tests {
		te.reset()
		te.stdin.WriteString(tt.stdin)
		if code := te.run(tt.args...); code != tt.code || !strings.Contains(te.stderr.String(), tt.want) {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, te.stderr)
		}
	}
}

// TestSendBadConfig checks that an unreadable config file fails send.
//
// Parameters:
//   - t: the test.
func TestSendBadConfig(t *testing.T) {
	te := newTestEnv(t)
	writeConfig(t, te, "{")
	te.stdin.WriteString("x")
	if code := te.run("send"); code != exitIO {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
}

// TestSendServerError checks that a server failure surfaces as a network
// error and prints no link.
//
// Parameters:
//   - t: the test.
func TestSendServerError(t *testing.T) {
	te := newTestEnv(t)
	srv := newTLS(t, te, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	te.stdin.WriteString("x")
	if code := te.run("send", "--server", srv.URL); code == exitOK || te.stdout.Len() != 0 {
		t.Fatalf("exit %d, stdout %q", code, te.stdout)
	}
}
