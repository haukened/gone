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

	"github.com/haukened/gone/v3/internal/domain"
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

// request makes a request on origin and records everything about it that
// must never be printed afterwards: the reply link's fill token and public
// key, the request ID, and the saved manage token and private key.
//
// Parameters:
//   - origin: server URL.
//
// Returns the reply link and request ID.
func (l *leakEnv) request(origin string) (string, string) {
	l.t.Helper()
	l.te.reset()
	raw := runJSON(l.t, l.te, exitOK, "request", "--json", "-s", origin)
	u, err := url.Parse(raw["link"])
	if err != nil {
		l.t.Fatal(err)
	}
	n := len(l.secrets)
	frag, _ := strings.CutPrefix(u.Fragment, "v3:")
	pub, fill, _ := strings.Cut(frag, ".")
	l.secrets[key("reply public key", n)] = pub
	l.secrets[key("fill token", n)] = fill
	l.secrets[key("request id", n)] = raw["id"]
	for _, r := range savedRows(l.t, l.te) {
		if r.ID == raw["id"] {
			l.secrets[key("request manage token", n)] = r.ManageToken
			l.secrets[key("private key hex", n)] = fmt.Sprintf("%x", r.PrivateKey)
			l.secrets[key("private key b64", n)] = domain.EncodeB64URL(r.PrivateKey)
		}
	}
	return raw["link"], raw["id"]
}

// replyArgs answers a request link read from stdin with the sentinel
// message and attachment.
//
// Returns the arguments.
func (l *leakEnv) replyArgs() []string {
	return []string{"reply", "-", "--message-file", l.msgFile, "-f", l.attFile}
}

// answered makes a request on the leak server and answers it.
//
// Returns the request ID.
func (l *leakEnv) answered() string {
	l.t.Helper()
	link, id := l.request(l.url)
	l.te.reset()
	l.te.stdin.WriteString(link)
	if code := l.te.run(l.replyArgs()...); code != exitOK {
		l.t.Fatalf("reply exit %d: %s", code, l.te.stderr)
	}
	return id
}

// openArgs opens a request's reply into fresh files.
//
// Parameters:
//   - id: request ID.
//
// Returns the arguments.
func (l *leakEnv) openArgs(id string) []string {
	return []string{"request", "open", id, "-o", l.t.TempDir(), "--message-out", filepath.Join(l.t.TempDir(), "m")}
}

// requestLeakCases cover request and reply failures and successes.
var requestLeakCases = []leakCase{
	{"reply to a cancelled request", func(l *leakEnv) (string, []string, int) {
		link, id := l.request(l.url)
		if code := l.te.run("request", "cancel", id); code != exitOK {
			l.t.Fatalf("cancel exit %d", code)
		}
		return link, l.replyArgs(), exitNotFound
	}},
	{"reply upload error", func(l *leakEnv) (string, []string, int) {
		link, _ := l.request(l.url)
		l.fault.Store(&fault{http.MethodPut, http.StatusInternalServerError})
		return link, l.replyArgs(), exitNetwork
	}},
	{"reply truncated link", func(l *leakEnv) (string, []string, int) {
		link, _ := l.request(l.url)
		return link[:len(link)-6], l.replyArgs(), exitUsage
	}},
	{"reply success", func(l *leakEnv) (string, []string, int) {
		link, _ := l.request(l.url)
		return link, l.replyArgs(), exitOK
	}},
	{"open claim error", func(l *leakEnv) (string, []string, int) {
		id := l.answered()
		l.fault.Store(&fault{http.MethodGet, http.StatusInternalServerError})
		return "", l.openArgs(id), exitNetwork
	}},
	{"open ack failure", func(l *leakEnv) (string, []string, int) {
		id := l.answered()
		l.fault.Store(&fault{http.MethodDelete, http.StatusInternalServerError})
		return "", l.openArgs(id), exitNetwork
	}},
	{"open success", func(l *leakEnv) (string, []string, int) {
		return "", l.openArgs(l.answered()), exitOK
	}},
	{"open not ready", func(l *leakEnv) (string, []string, int) {
		_, id := l.request(l.url)
		return "", l.openArgs(id), exitNotFound
	}},
	{"cancel server down", func(l *leakEnv) (string, []string, int) {
		srv := newTLS(l.t, l.te, newGoneHandler(l.t))
		_, id := l.request(srv.URL)
		srv.Close()
		return "", []string{"request", "cancel", id}, exitNetwork
	}},
}

// TestRequestListNeverPrintsKeys checks the request list: the text table
// shows no part of the reply link, and the JSON (which includes the link on
// purpose; it can't open anything) never shows the manage token or key.
//
// Parameters:
//   - t: the test.
func TestRequestListNeverPrintsKeys(t *testing.T) {
	l := newLeakEnv(t)
	l.answered()
	l.te.reset()
	if code := l.te.run("request", "list"); code != exitOK {
		t.Fatalf("list exit %d", code)
	}
	requireNoLeak(t, "list text", l)
	for k := range l.secrets {
		if strings.HasPrefix(k, "reply public key") || strings.HasPrefix(k, "fill token") || strings.HasPrefix(k, "request id") {
			delete(l.secrets, k)
		}
	}
	l.te.reset()
	if code := l.te.run("request", "list", "--json"); code != exitOK {
		t.Fatalf("list --json exit %d", code)
	}
	requireNoLeak(t, "list json", l)
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
	for _, tc := range append(append([]leakCase{}, leakCases...), requestLeakCases...) {
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
