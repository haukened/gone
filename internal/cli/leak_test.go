package cli

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// fault makes the leak server answer one HTTP method with a status.
type fault struct {
	method string
	code   int
}

// leakEnv runs CLI failure paths with known secrets and checks that none
// of them ever reach stdout or stderr.
type leakEnv struct {
	t        *testing.T
	te       *testEnv
	url      string
	fault    atomic.Pointer[fault]
	secrets  map[string]string
	msgFile  string
	passFile string
	attFile  string
}

// newLeakEnv starts a fault-injecting server and writes the sentinel
// message, passphrase and attachment files.
//
// Parameters:
//   - t: the test.
//
// Returns the environment.
func newLeakEnv(t *testing.T) *leakEnv {
	t.Helper()
	l := &leakEnv{t: t, te: newTestEnv(t), secrets: map[string]string{
		"message":    "MSG-" + rand.Text(),
		"passphrase": "PASS-" + rand.Text(),
		"attachment": "ATT-" + rand.Text(),
	}}
	inner := newGoneHandler(t)
	srv := newTLS(t, l.te, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f := l.fault.Load(); f != nil && f.method == r.Method {
			http.Error(w, "fault", f.code)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	l.url = srv.URL
	l.msgFile = writeTemp(t, "msg", l.secrets["message"])
	l.passFile = writeTemp(t, "pass", l.secrets["passphrase"]+"\n")
	l.attFile = writeTemp(t, "att.txt", l.secrets["attachment"])
	return l
}

// send stores a v2 secret with the sentinel message, passphrase and
// attachment on origin and records its link secrets.
//
// Parameters:
//   - origin: server URL.
//
// Returns the link and manage link.
func (l *leakEnv) send(origin string) (string, string) {
	l.t.Helper()
	l.te.reset()
	raw := runJSON(l.t, l.te, exitOK, "send", "--json", "-s", origin, "--message-file", l.msgFile,
		"--passphrase-file", l.passFile, "-f", l.attFile)
	l.record(raw["link"], raw["manage_link"])
	return raw["link"], raw["manage_link"]
}

// record adds a link's fragment, key, secret ID and (when given) manage
// token to the secrets that must never be printed.
//
// Parameters:
//   - link: share link.
//   - manage: manage link, or "".
func (l *leakEnv) record(link, manage string) {
	l.t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		l.t.Fatal(err)
	}
	n := len(l.secrets)
	l.secrets[key("fragment", n)] = u.Fragment
	_, k, _ := strings.Cut(u.Fragment, ":")
	l.secrets[key("key", n)] = k
	l.secrets[key("id", n)] = path.Base(u.Path)
	if manage != "" {
		_, token, _ := strings.Cut(manage, "#")
		l.secrets[key("manage token", n)] = token
	}
}

// key names a recorded secret uniquely.
//
// Parameters:
//   - name: secret kind.
//   - n: sequence number.
//
// Returns the map key.
func key(name string, n int) string { return fmt.Sprintf("%s %d", name, n) }

// getArgs returns get arguments that read the link from stdin and write
// the message to a fresh file.
//
// Parameters:
//   - extra: further arguments.
//
// Returns the arguments.
func (l *leakEnv) getArgs(extra ...string) []string {
	args := []string{"get", "-", "--passphrase-file", l.passFile, "-o", l.t.TempDir(),
		"--message-out", filepath.Join(l.t.TempDir(), "m")}
	return append(args, extra...)
}

// deadServer starts a server, stores a secret on it, and shuts it down.
//
// Returns the link and manage link of the stranded secret.
func (l *leakEnv) deadServer() (string, string) {
	l.t.Helper()
	srv := newTLS(l.t, l.te, newGoneHandler(l.t))
	link, manage := l.send(srv.URL)
	srv.Close()
	return link, manage
}

// leakCase is one failure path: setup returns stdin, arguments and the
// expected exit code.
type leakCase struct {
	name  string
	setup func(l *leakEnv) (string, []string, int)
}

// leakCases covers send, get and manage failures plus a successful
// --message-out get.
var leakCases = []leakCase{
	{"send server error", func(l *leakEnv) (string, []string, int) {
		l.fault.Store(&fault{http.MethodPost, http.StatusInternalServerError})
		return "", l.sendArgs(), exitNetwork
	}},
	{"send rate limited", func(l *leakEnv) (string, []string, int) {
		l.fault.Store(&fault{http.MethodPost, http.StatusTooManyRequests})
		return "", l.sendArgs(), exitRateLimited
	}},
	{"send server down", func(l *leakEnv) (string, []string, int) {
		srv := newTLS(l.t, l.te, newGoneHandler(l.t))
		srv.Close()
		return "", []string{"send", "-s", srv.URL, "--message-file", l.msgFile, "--passphrase-file", l.passFile}, exitNetwork
	}},
	{"send message file is a directory", func(l *leakEnv) (string, []string, int) {
		return "", []string{"send", "-s", l.url, "--message-file", l.t.TempDir(), "--passphrase-file", l.passFile}, exitUsage
	}},
	{"wrong passphrase", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		wrong := writeTemp(l.t, "wrong", "WRONG-"+rand.Text())
		return link, []string{"get", "-", "--passphrase-file", wrong, "--message-out", filepath.Join(l.t.TempDir(), "m")}, exitPassphrase
	}},
	{"truncated link", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		cut := link[:len(link)-8]
		_, frag, _ := strings.Cut(cut, "#")
		l.secrets[key("truncated fragment", len(l.secrets))] = frag
		return cut, l.getArgs(), exitUsage
	}},
	{"message-out exists", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		return link, []string{"get", "-", "--passphrase-file", l.passFile, "--message-out", writeTemp(l.t, "m", "")}, exitIO
	}},
	{"claim server error", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		l.fault.Store(&fault{http.MethodGet, http.StatusInternalServerError})
		return link, l.getArgs(), exitNetwork
	}},
	{"get server down", func(l *leakEnv) (string, []string, int) {
		link, _ := l.deadServer()
		return link, l.getArgs(), exitNetwork
	}},
	{"corrupt envelope", func(l *leakEnv) (string, []string, int) {
		plain := append(envelope.Magic[:], []byte(l.secrets["message"])...)
		link := createRawV1(l.t, l.te, l.url, plain)
		l.record(link, "")
		return link, []string{"get", "-", "--message-out", filepath.Join(l.t.TempDir(), "m")}, exitIntegrity
	}},
	{"attachment rollback", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		out := l.t.TempDir()
		fillCollisions(l.t, out, filepath.Base(l.attFile))
		return link, []string{"get", "-", "--passphrase-file", l.passFile, "-o", out,
			"--message-out", filepath.Join(l.t.TempDir(), "m")}, exitIO
	}},
	{"ack failure", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		l.fault.Store(&fault{http.MethodDelete, http.StatusInternalServerError})
		return link, l.getArgs(), exitNetwork
	}},
	{"get success", func(l *leakEnv) (string, []string, int) {
		link, _ := l.send(l.url)
		return link, l.getArgs(), exitOK
	}},
	{"status after revoke", func(l *leakEnv) (string, []string, int) {
		_, manage := l.send(l.url)
		l.te.reset()
		l.te.stdin.WriteString(manage)
		if code := l.te.run("revoke", "-"); code != exitOK {
			l.t.Fatalf("revoke exit %d", code)
		}
		return manage, []string{"status", "-"}, exitNotFound
	}},
	{"status server down", func(l *leakEnv) (string, []string, int) {
		_, manage := l.deadServer()
		return manage, []string{"status", "-"}, exitNetwork
	}},
	{"revoke server down", func(l *leakEnv) (string, []string, int) {
		_, manage := l.deadServer()
		return manage, []string{"revoke", "-"}, exitNetwork
	}},
	{"malformed manage link", func(l *leakEnv) (string, []string, int) {
		_, manage := l.send(l.url)
		cut := manage[:len(manage)-5]
		_, token, _ := strings.Cut(cut, "#")
		l.secrets[key("truncated token", len(l.secrets))] = token
		return cut, []string{"status", "-"}, exitUsage
	}},
}

// sendArgs returns send arguments that take every secret from files.
//
// Returns the arguments.
func (l *leakEnv) sendArgs() []string {
	return []string{"send", "-s", l.url, "--message-file", l.msgFile, "--passphrase-file", l.passFile, "-f", l.attFile}
}

// TestNoSecretInOutput runs every failure path, in text and JSON mode,
// with known secrets and checks that no plaintext, passphrase, link
// fragment or key, manage token or secret ID is ever printed.
//
// Parameters:
//   - t: the test.
func TestNoSecretInOutput(t *testing.T) {
	for _, tc := range leakCases {
		for _, asJSON := range []bool{false, true} {
			l := newLeakEnv(t)
			stdin, args, want := tc.setup(l)
			if asJSON {
				args = append(args, "--json")
			}
			l.te.reset()
			l.te.stdin.WriteString(stdin)
			if code := l.te.run(args...); code != want {
				t.Fatalf("%s (json=%v): exit %d, want %d: %s", tc.name, asJSON, code, want, l.te.stderr)
			}
			requireNoLeak(t, tc.name, l)
		}
	}
}

// requireNoLeak fails if any recorded secret appears in stdout or stderr.
//
// Parameters:
//   - t: the test.
//   - name: case name for the report.
//   - l: environment with captured output and secrets.
func requireNoLeak(t *testing.T, name string, l *leakEnv) {
	t.Helper()
	out := l.te.stdout.String() + l.te.stderr.String()
	for what, secret := range l.secrets {
		if secret == "" {
			t.Fatalf("%s: empty secret %s", name, what)
		}
		if strings.Contains(out, secret) {
			t.Errorf("%s: %s leaked in %q", name, what, out)
		}
	}
}
