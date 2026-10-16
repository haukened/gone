package cli

import (
	"net/http"
	"strings"
	"testing"

	gonecfg "github.com/haukened/gone/v3/internal/config"
)

// newPolicyServer starts a real Gone server with an operator policy.
//
// Parameters:
//   - t: the test.
//   - te: environment whose RootCAs are set to trust the server.
//   - maxFiles: GONE_MAX_FILES.
//
// Returns the server's URL.
func newPolicyServer(t *testing.T, te *testEnv, maxFiles int) string {
	t.Helper()
	cfg := gonecfg.DefaultAppConfig
	cfg.RequirePassphrase, cfg.MaxFiles = true, maxFiles
	return newTLS(t, te, newGoneHandlerWith(t, cfg)).URL
}

// TestSendPolicy checks send honours the server's policy before encrypting.
func TestSendPolicy(t *testing.T) {
	te := newTestEnv(t)
	url := newPolicyServer(t, te, 1)
	a, b := writeTemp(t, "a.txt", "a"), writeTemp(t, "b.txt", "b")
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no passphrase", []string{"send", "-s", url}, exitUsage, "requires a passphrase"},
		{"no passphrase json", []string{"send", "-s", url, "--json"}, exitUsage, `"error":"policy"`},
		{"too many files", []string{"send", "-s", url, "--passphrase-generate", "-f", a, "-f", b}, exitUsage, "at most 1"},
		{"allowed", []string{"send", "-s", url, "--passphrase-generate", "-f", a}, exitOK, ""},
	}
	for _, tt := range tests {
		te.reset()
		te.stdin.WriteString("x")
		if code := te.run(tt.args...); code != tt.code || !strings.Contains(te.stderr.String(), tt.want) {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, te.stderr)
		}
	}
}

// TestReplyPolicy checks replies keep working on a passphrase-only server
// (they are exempt) but honour its file limit.
func TestReplyPolicy(t *testing.T) {
	te := newTestEnv(t)
	url := newPolicyServer(t, te, 0)
	link := requestJSON(t, te, url)["link"]
	r := replier(t, te)
	if code := replyWith(r, link, "x", "-f", writeTemp(t, "a.txt", "a")); code != exitUsage || !strings.Contains(r.stderr.String(), "does not accept files") {
		t.Fatalf("reply with file: exit %d %s", code, r.stderr)
	}
	if code := replyWith(r, link, "hunter2"); code != exitOK {
		t.Fatalf("reply: exit %d %s", code, r.stderr)
	}
}

// TestSendPolicyOldServer checks a server without /api/policy is treated as
// having none, and that a 403 from create still reads as the passphrase rule.
func TestSendPolicyOldServer(t *testing.T) {
	te := newTestEnv(t)
	srv := newTLS(t, te, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	}))
	te.stdin.WriteString("x")
	if code := te.run("send", "-s", srv.URL); code != exitUsage || !strings.Contains(te.stderr.String(), "requires a passphrase") {
		t.Fatalf("exit %d, stderr %q", code, te.stderr)
	}
}
