package integration_test

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// exitUsage is the documented exit code for a refused link origin.
const exitUsage = 2

// originSetup sends a secret to a real server and starts an attacker
// server that counts requests. The runner starts with no GONE_SERVER and
// no config, as on first run.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - r: the runner.
//   - server: the real server's origin.
//   - link: a share link on it.
//   - manage: its manage link.
//   - attacker: the attacker server's origin.
//   - hits: the attacker's request count.
func originSetup(t *testing.T) (r *cliRunner, server, link, manage, attacker string, hits *atomic.Int64) {
	t.Helper()
	srv := newGone(t)
	r = newCLI(t, srv)
	link, manage = r.send(t, "pinned", "--server", srv.URL)
	delete(r.vars, "GONE_SERVER")
	hits = &atomic.Int64{}
	att := newTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	return r, srv.URL, link, manage, att.URL, hits
}

// TestCLIOriginMismatchRefused checks that a link pointing at another
// server is refused before any request reaches it, and that the message
// names both origins and how to trust one, but not the link.
//
// Parameters:
//   - t: the test.
func TestCLIOriginMismatchRefused(t *testing.T) {
	r, server, link, manage, attacker, hits := originSetup(t)
	r.vars["GONE_SERVER"] = server
	for _, args := range [][]string{
		{"get", attacker + strings.TrimPrefix(link, server)},
		{"status", attacker + strings.TrimPrefix(manage, server)},
		{"revoke", attacker + strings.TrimPrefix(manage, server)},
	} {
		code := r.run("", args...)
		msg := r.stderr.String()
		if code != exitUsage || !strings.Contains(msg, attacker) || !strings.Contains(msg, server+" (from GONE_SERVER)") ||
			!strings.Contains(msg, "--allow-origin "+attacker) || strings.Contains(msg, "/secret/") || strings.Contains(msg, "#v1:") {
			t.Errorf("%s: exit %d, stderr %q", args[0], code, msg)
		}
	}
	delete(r.vars, "GONE_SERVER")
	if code := r.run("", "get", link); code != exitUsage || !strings.Contains(r.stderr.String(), "gone set server "+server) {
		t.Errorf("first run: exit %d, stderr %q", code, r.stderr)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("attacker server saw %d requests", n)
	}
	// The secret was never touched: it still opens once trusted.
	if code := r.run("", "get", link, "--allow-origin", server); code != 0 || r.stdout.String() != "pinned" {
		t.Fatalf("get after refusals: exit %d: %s", code, r.stderr)
	}
}

// TestCLIOriginAllowFlag checks that --allow-origin trusts a server for
// one command only.
//
// Parameters:
//   - t: the test.
func TestCLIOriginAllowFlag(t *testing.T) {
	r, server, _, manage, _, _ := originSetup(t)
	if code := r.run("", "status", manage, "--allow-origin", "https://other.example,"+server); code != 0 {
		t.Fatalf("allowed: exit %d: %s", code, r.stderr)
	}
	if code := r.run("", "status", manage); code != exitUsage {
		t.Fatalf("not allowed: exit %d", code)
	}
}

// TestCLIOriginConfigServer checks that the server saved with "gone set
// server" is trusted.
//
// Parameters:
//   - t: the test.
func TestCLIOriginConfigServer(t *testing.T) {
	r, server, link, _, _, _ := originSetup(t)
	if code := r.run("", "set", "server", server); code != 0 {
		t.Fatalf("set exit %d: %s", code, r.stderr)
	}
	if code := r.run("", "get", link); code != 0 || r.stdout.String() != "pinned" {
		t.Fatalf("get exit %d: %s", code, r.stderr)
	}
}

// TestCLIOriginEnvServer checks that $GONE_SERVER is trusted.
//
// Parameters:
//   - t: the test.
func TestCLIOriginEnvServer(t *testing.T) {
	r, server, link, _, _, _ := originSetup(t)
	r.vars["GONE_SERVER"] = server
	if code := r.run("", "get", link); code != 0 || r.stdout.String() != "pinned" {
		t.Fatalf("get exit %d: %s", code, r.stderr)
	}
}
