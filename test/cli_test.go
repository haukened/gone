package integration_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/haukened/gone/internal/client"
	"github.com/haukened/gone/internal/ratelimit"
)

// Exit codes documented in docs/cli.md.
const (
	exitNotFound    = 3
	exitPassphrase  = 4
	exitRateLimited = 5
	exitNetwork     = 7
)

// newGone starts the real gone server over TLS without rate limits.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - *httptest.Server: the running server.
func newGone(t *testing.T) *httptest.Server {
	t.Helper()
	return newTLSServer(t, newHandler(t, &fakeClock{t: time.Now()}, limits{}))
}

// TestCLITextRoundTrip sends a v1 message and reads it back once.
//
// Parameters:
//   - t: the test.
func TestCLITextRoundTrip(t *testing.T) {
	srv := newGone(t)
	r := newCLI(t, srv)
	link, _ := r.send(t, "line one\nline two", "--server", srv.URL)
	if !strings.Contains(link, "#v1:") {
		t.Fatalf("link %q", link)
	}
	if code := r.run("", "get", link); code != 0 || r.stdout.String() != "line one\nline two" {
		t.Fatalf("get exit %d: %q %s", code, r.stdout, r.stderr)
	}
	if code := r.run("", "get", link); code != exitNotFound {
		t.Fatalf("second get exit %d", code)
	}
}

// TestCLIPassphraseAndAttachments sends a v2 secret with two attachments
// of the same name and checks the wrong and right passphrases.
//
// Parameters:
//   - t: the test.
func TestCLIPassphraseAndAttachments(t *testing.T) {
	srv := newGone(t)
	r := newCLI(t, srv)
	att := writeFile(t, "report.csv", "a,b\n1,2\n")
	pass := writeFile(t, "pass", "correct horse battery")
	wrong := writeFile(t, "wrong", "incorrect horse battery")
	args := []string{"--server", srv.URL, "--file", att, "--file", att, "--passphrase-file", pass}

	link, _ := r.send(t, "", args...)
	if code := r.run("", "get", link, "--passphrase-file", wrong, "-o", t.TempDir()); code != exitPassphrase {
		t.Fatalf("wrong passphrase exit %d: %s", code, r.stderr)
	}

	link, _ = r.send(t, "see attached", args...)
	out := t.TempDir()
	if code := r.run("", "get", link, "--passphrase-file", pass, "-o", out); code != 0 {
		t.Fatalf("get exit %d: %s", code, r.stderr)
	}
	if r.stdout.String() != "see attached" {
		t.Fatalf("message %q", r.stdout)
	}
	for _, name := range []string{"report.csv", "report (1).csv"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(data) != "a,b\n1,2\n" {
			t.Fatalf("%s: %q %v", name, data, err)
		}
	}
}

// TestCLIStatusAndRevoke checks the manage link lifecycle against the real
// server.
//
// Parameters:
//   - t: the test.
func TestCLIStatusAndRevoke(t *testing.T) {
	srv := newGone(t)
	r := newCLI(t, srv)
	link, manage := r.send(t, "x", "--server", srv.URL)
	if code := r.run("", "status", manage); code != 0 || r.field(t, "State:") != "pending" {
		t.Fatalf("status exit %d: %q %s", code, r.stdout, r.stderr)
	}
	if code := r.run("", "revoke", manage); code != 0 {
		t.Fatalf("revoke exit %d: %s", code, r.stderr)
	}
	if code := r.run("", "get", link); code != exitNotFound {
		t.Fatalf("get after revoke exit %d", code)
	}
	if code := r.run("", "revoke", manage); code != exitNotFound {
		t.Fatalf("second revoke exit %d", code)
	}
}

// TestCLIRateLimited checks that a 429 maps to exit 5 with the server's
// Retry-After hint and no automatic retry.
//
// Parameters:
//   - t: the test.
func TestCLIRateLimited(t *testing.T) {
	lim := limits{create: ratelimit.Rate{Count: 1, Per: time.Hour}, burst: 1}
	srv := newTLSServer(t, newHandler(t, &fakeClock{t: time.Now()}, lim))
	r := newCLI(t, srv)
	r.send(t, "first", "--server", srv.URL)
	if code := r.run("second", "send", "--server", srv.URL); code != exitRateLimited {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if !strings.Contains(r.stderr.String(), "retry after") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

// TestCLIRefusesRedirect checks that the CLI never follows a redirect,
// so a secret cannot be steered to another origin.
//
// Parameters:
//   - t: the test.
func TestCLIRefusesRedirect(t *testing.T) {
	target := newGone(t)
	var hops atomic.Int32
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	r := newCLI(t, srv)
	if code := r.run("x", "send", "--server", srv.URL); code != exitNetwork {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
	if hops.Load() != 1 || r.stdout.Len() != 0 {
		t.Fatalf("hops %d, stdout %q", hops.Load(), r.stdout)
	}
}

// TestCLIRejectsOversizeResponse checks that a claim response declaring
// more than the client's ciphertext limit is refused before reading.
//
// Parameters:
//   - t: the test.
func TestCLIRejectsOversizeResponse(t *testing.T) {
	inner := newHandler(t, &fakeClock{t: time.Now()}, limits{})
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/secret/") {
			inner.ServeHTTP(w, r)
			return
		}
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Length", strconv.FormatInt(client.MaxCiphertext+1, 10))
		w.WriteHeader(rec.Code)
	}))
	r := newCLI(t, srv)
	link, _ := r.send(t, "x", "--server", srv.URL)
	if code := r.run("", "get", link); code != exitNetwork || !strings.Contains(r.stderr.String(), "larger") {
		t.Fatalf("exit %d: %s", code, r.stderr)
	}
}
