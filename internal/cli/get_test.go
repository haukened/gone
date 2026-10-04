package cli

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/client"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/envelope"
)

// createRawV1 stores plain as a v1 secret on origin without going through
// the payload encoder, so tests can plant malformed plaintext.
//
// Parameters:
//   - t: the test.
//   - te: environment that trusts the server.
//   - origin: server origin.
//   - plain: exact plaintext to encrypt.
//
// Returns the share link.
func createRawV1(t *testing.T, te *testEnv, origin string, plain []byte) string {
	t.Helper()
	cl, err := client.New(origin, client.Options{RootCAs: te.RootCAs, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := envelope.NewKey()
	nonce, body, err := envelope.Seal(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cl.Create(context.Background(), client.CreateRequest{Version: domain.ProtocolV1, Nonce: nonce, TTL: time.Hour, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := buildLinks(origin, res.ID, sealed{version: domain.ProtocolV1, key: key}, res.ManageToken)
	if err != nil {
		t.Fatal(err)
	}
	return link
}

// TestGetLocalErrors checks failures detected before any network call.
//
// Parameters:
//   - t: the test.
func TestGetLocalErrors(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link, _ := sendText(t, te, "hi", "--server", srv.URL)
	httpLink := "http" + strings.TrimPrefix(link, "https")
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no link", []string{"get"}, exitUsage, "link"},
		{"bad link", []string{"get", "https://example.com/nope"}, exitUsage, "not a valid gone link"},
		{"http link", []string{"get", httpLink}, exitUsage, "--insecure"},
		{"bad timeout", []string{"get", "--timeout", "0s", link}, exitUsage, "timeout"},
		{"v1 with passphrase file", []string{"get", "--passphrase-file", writeTemp(t, "p", "x"), link}, exitUsage, "no passphrase"},
		{"missing out dir", []string{"get", "-o", filepath.Join(t.TempDir(), "nope"), link}, exitIO, "output directory"},
	}
	for _, tt := range tests {
		te.reset()
		if code := te.run(tt.args...); code != tt.code || !strings.Contains(te.stderr.String(), tt.want) {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, te.stderr)
		}
	}
	te.reset()
	if code := te.run("get", link); code != exitOK || te.stdout.String() != "hi" {
		t.Fatalf("link was consumed by a local error: %d %q %q", code, te.stdout, te.stderr)
	}
}

// TestGetNotFound checks the exit code for a missing secret.
//
// Parameters:
//   - t: the test.
func TestGetNotFound(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link, manage := sendText(t, te, "hi", "--server", srv.URL)
	te.reset()
	if code := te.run("revoke", manage); code != exitOK {
		t.Fatalf("revoke: %d %s", code, te.stderr)
	}
	te.reset()
	if code := te.run("get", link); code != exitNotFound {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
}

// TestGetAckFailure checks that a failed acknowledgement still delivers the
// secret but exits with a network error.
//
// Parameters:
//   - t: the test.
func TestGetAckFailure(t *testing.T) {
	te := newTestEnv(t)
	inner := newGoneHandler(t)
	srv := newTLS(t, te, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	link, _ := sendText(t, te, "hi", "--server", srv.URL)
	te.reset()
	if code := te.run("get", "--json", link); code != exitNetwork {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	if !strings.Contains(te.stdout.String(), `"message":"hi"`) || !strings.Contains(te.stderr.String(), "not_confirmed") {
		t.Fatalf("stdout %q stderr %q", te.stdout, te.stderr)
	}
}

// TestGetCorruptEnvelope checks that malformed GONE2 plaintext is an
// integrity failure and writes nothing.
//
// Parameters:
//   - t: the test.
func TestGetCorruptEnvelope(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link := createRawV1(t, te, srv.URL, append(envelope.Magic[:], 0xff, 0xff, 0xff, 0xff))
	out := t.TempDir()
	if code := te.run("get", "-o", out, link); code != exitIntegrity {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Fatalf("files written: %v", entries)
	}
}

// TestGetJSONWriteError checks that a stdout failure is an I/O error.
//
// Parameters:
//   - t: the test.
func TestGetJSONWriteError(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link, _ := sendText(t, te, "hi", "--server", srv.URL)
	te.reset()
	te.Stdout = errWriter{}
	if code := te.run("get", "--json", link); code != exitIO {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
}

// TestGetTerminalOutput checks escaping, --raw and the trailing newline when
// stdout is a terminal.
//
// Parameters:
//   - t: the test.
func TestGetTerminalOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  bool
		want string
	}{
		{"escaped", false, "a\\u001b[31mb\n"},
		{"raw", true, "a\x1b[31mb\n"},
	}
	for _, tt := range tests {
		te := newTestEnv(t)
		srv := newGoneServer(t, te)
		link, _ := sendText(t, te, "a\x1b[31mb", "--server", srv.URL)
		te.reset()
		te.StdoutTTY = true
		args := []string{"get", link}
		if tt.raw {
			args = []string{"get", "--raw", link}
		}
		if code := te.run(args...); code != exitOK || te.stdout.String() != tt.want {
			t.Errorf("%s: exit %d, stdout %q", tt.name, code, te.stdout)
		}
	}
}

// TestGetSavesFilesHumanMode checks that attachments are reported on
// stderr and the message goes to stdout without --json.
//
// Parameters:
//   - t: the test.
func TestGetSavesFilesHumanMode(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	att := writeTemp(t, "a.txt", "body")
	link, _ := sendText(t, te, "msg", "--server", srv.URL, "--file", att)
	out := t.TempDir()
	te.reset()
	if code := te.run("get", link, "-o", out); code != exitOK {
		t.Fatalf("get exit %d: %s", code, te.stderr)
	}
	want := "Saved: " + filepath.Join(out, "a.txt") + " (4 bytes)"
	if te.stdout.String() != "msg" || !strings.Contains(te.stderr.String(), want) {
		t.Fatalf("stdout %q, stderr %q", te.stdout, te.stderr)
	}
}
