package cli

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

// TestMain silences the server's request logs.
//
// Parameters:
//   - m: the test runner.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// sendText runs "gone send" with msg on stdin and returns the link and
// manage link from text output.
//
// Parameters:
//   - t: the test.
//   - te: environment.
//   - msg: message to pipe on stdin.
//   - args: extra send arguments.
//
// Returns the link and the manage link.
func sendText(t *testing.T, te *testEnv, msg string, args ...string) (string, string) {
	t.Helper()
	te.reset()
	te.stdin.WriteString(msg)
	if code := te.run(append([]string{"send"}, args...)...); code != exitOK {
		t.Fatalf("send exit %d: %s", code, te.stderr)
	}
	out := te.stdout.String()
	return linkLine(t, out, "Link:"), linkLine(t, out, "Manage link:")
}

// TestSendGetRoundTripV1 checks a plain v1 send/get round trip and the
// printed links and expiry.
//
// Parameters:
//   - t: the test.
func TestSendGetRoundTripV1(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link, manage := sendText(t, te, "hello\nworld", "--server", srv.URL, "--ttl", "10m")
	requireRoundTripV1Links(t, te, srv.URL, link, manage)
	requireGetOnce(t, te, link, "hello\nworld")
}

// requireRoundTripV1Links verifies text send links and expiry output.
//
// Parameters:
//   - t: the test handle.
//   - te: environment with captured output.
//   - serverURL: expected server URL prefix.
//   - link: printed secret link.
//   - manage: printed manage link.
func requireRoundTripV1Links(t *testing.T, te *testEnv, serverURL string, link string, manage string) {
	t.Helper()
	if !strings.HasPrefix(link, serverURL+"/secret/") || !strings.Contains(link, "#v1:") {
		t.Fatalf("link = %q", link)
	}
	if !strings.HasPrefix(manage, serverURL+"/manage/") {
		t.Fatalf("manage = %q", manage)
	}
	if !strings.Contains(te.stdout.String(), "Expires:") {
		t.Fatalf("missing expiry: %q", te.stdout)
	}
}

// requireGetOnce verifies that a link opens once and is then gone.
//
// Parameters:
//   - t: the test handle.
//   - te: environment used to run the CLI.
//   - link: secret link to fetch.
//   - want: expected first retrieval body.
func requireGetOnce(t *testing.T, te *testEnv, link string, want string) {
	t.Helper()
	te.reset()
	if code := te.run("get", link); code != exitOK {
		t.Fatalf("get exit %d: %s", code, te.stderr)
	}
	if got := te.stdout.String(); got != want {
		t.Fatalf("message = %q", got)
	}

	te.reset()
	if code := te.run("get", link); code != exitNotFound {
		t.Fatalf("second get exit %d: %s", code, te.stderr)
	}
}

// TestSendGetJSONWithFilesAndPassphrase checks a v2 round trip with
// attachments, a passphrase file and JSON output on both sides.
//
// Parameters:
//   - t: the test.
func TestSendGetJSONWithFilesAndPassphrase(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	pf := writeTemp(t, "pass.txt", "correct horse battery\n")
	att := writeTemp(t, "notes.txt", "file body")
	te.stdin.WriteString("msg")
	raw := runJSON(t, te, exitOK, "send", "--json", "-s", srv.URL, "-f", att, "--file", att, "--passphrase-file", pf)
	checkSendJSON(t, raw, "#v2:")

	out := t.TempDir()
	te.reset()
	got := getJSON(t, te, raw["link"], "--json", "-o", out, "--passphrase-file", pf)
	if got.Message != "msg" || len(got.Files) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got.Files[1].Path != filepath.Join(out, "notes (1).txt") || got.Files[0].Type != "text/plain" {
		t.Fatalf("files %+v", got.Files)
	}
	for _, f := range got.Files {
		checkSavedFile(t, f.Path, "file body")
	}
}

// TestGetPromptsForPassphraseWithRetries checks that a wrong terminal
// passphrase is retried and a correct one then opens the secret.
//
// Parameters:
//   - t: the test.
func TestGetPromptsForPassphraseWithRetries(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	te.StdinTTY = true
	te.reset()
	te.passwords = []string{"longenough", "longenough"}
	te.stdin.WriteString("secret")
	if code := te.run("send", "-s", srv.URL, "--passphrase-prompt"); code != exitOK {
		t.Fatalf("send exit %d: %s", code, te.stderr)
	}
	if !strings.Contains(te.stderr.String(), messagePrompt) {
		t.Fatalf("no message prompt: %q", te.stderr)
	}
	link := linkLine(t, te.stdout.String(), "Link:")

	te.reset()
	te.StdoutTTY = true
	te.passwords = []string{"wrong-one", "longenough"}
	if code := te.run("get", link, "-o", t.TempDir()); code != exitOK {
		t.Fatalf("get exit %d: %s", code, te.stderr)
	}
	if !strings.Contains(te.stderr.String(), "Wrong passphrase, try again.") {
		t.Fatalf("stderr %q", te.stderr)
	}
	if te.stdout.String() != "secret\n" {
		t.Fatalf("stdout %q", te.stdout)
	}
}

// TestGetWrongPassphraseExhaustsAttempts checks that repeated wrong
// passphrases exit with the passphrase code.
//
// Parameters:
//   - t: the test.
func TestGetWrongPassphraseExhaustsAttempts(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	pf := writeTemp(t, "p", "longenough")
	link, _ := sendText(t, te, "x", "-s", srv.URL, "--passphrase-file", pf)

	te.reset()
	te.StdinTTY = true
	te.passwords = []string{"bad-1", "bad-2", "bad-3"}
	if code := te.run("get", link, "-o", t.TempDir()); code != exitPassphrase {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	if n := strings.Count(te.stderr.String(), "Wrong passphrase, try again."); n != 2 {
		t.Fatalf("retries = %d: %q", n, te.stderr)
	}

	link, _ = sendText(t, te, "x", "-s", srv.URL, "--passphrase-file", pf)
	te.reset()
	bad := writeTemp(t, "bad", "nope")
	if code := te.run("get", link, "--passphrase-file", bad, "-o", t.TempDir()); code != exitPassphrase {
		t.Fatalf("file exit %d: %s", code, te.stderr)
	}
}

// TestGetInvalidPromptEntryRetries checks that an empty, oversized, or
// non-UTF-8 entry at the get prompt uses up an attempt instead of ending the
// command: the secret is already claimed, so exiting would lose it.
//
// Parameters:
//   - t: the test.
func TestGetInvalidPromptEntryRetries(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	pf := writeTemp(t, "p", "longenough")
	link, _ := sendText(t, te, "kept", "-s", srv.URL, "--passphrase-file", pf)

	te.reset()
	te.StdinTTY = true
	te.passwords = []string{"", "longenough"}
	if code := te.run("get", link, "-o", t.TempDir()); code != exitOK {
		t.Fatalf("get exit %d: %s", code, te.stderr)
	}
	if !strings.Contains(te.stderr.String(), "Invalid passphrase: ") || !strings.Contains(te.stderr.String(), ", try again.") {
		t.Fatalf("stderr %q", te.stderr)
	}
	if te.stdout.String() != "kept" {
		t.Fatalf("stdout %q", te.stdout)
	}

	link, _ = sendText(t, te, "x", "-s", srv.URL, "--passphrase-file", pf)
	te.reset()
	te.StdinTTY = true
	te.passwords = []string{"", strings.Repeat("a", domain.V2MaxPassphraseBytes+1), "\xff"}
	if code := te.run("get", link, "-o", t.TempDir()); code != exitPassphrase {
		t.Fatalf("exit %d, want %d: %s", code, exitPassphrase, te.stderr)
	}
	if n := strings.Count(te.stderr.String(), "Invalid passphrase: "); n != 2 {
		t.Fatalf("retries = %d: %q", n, te.stderr)
	}
}

// TestSendGeneratedPassphrase checks that --passphrase-generate prints a
// passphrase that opens the secret.
//
// Parameters:
//   - t: the test.
func TestSendGeneratedPassphrase(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	te.stdin.WriteString("x")
	if code := te.run("send", "-s", srv.URL, "--passphrase-generate"); code != exitOK {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	pass := linkLine(t, te.stderr.String(), "Passphrase (share it separately from the link):")
	link := linkLine(t, te.stdout.String(), "Link:")
	te.reset()
	pf := writeTemp(t, "p", pass)
	if code := te.run("get", link, "--passphrase-file", pf); code != exitOK || te.stdout.String() != "x" {
		t.Fatalf("get exit %d: %s %q", code, te.stderr, te.stdout)
	}
}

// TestSendUsesEnvAndConfigServers checks that GONE_SERVER and the saved
// config select the server when --server is absent.
//
// Parameters:
//   - t: the test.
func TestSendUsesEnvAndConfigServers(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	te.vars[serverEnv] = srv.URL
	if link, _ := sendText(t, te, "a"); !strings.HasPrefix(link, srv.URL) {
		t.Fatalf("env link %q", link)
	}
	delete(te.vars, serverEnv)
	te.reset()
	if code := te.run("set", "server", srv.URL); code != exitOK {
		t.Fatalf("set exit %d: %s", code, te.stderr)
	}
	if link, _ := sendText(t, te, "a"); !strings.HasPrefix(link, srv.URL) {
		t.Fatalf("config link %q", link)
	}
}
