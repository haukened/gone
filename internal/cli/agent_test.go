package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestAgentRoundTrip checks file in, link out and link in, file out, for
// v1 and v2 links in text and JSON modes: the message never reaches
// stdout or stderr, the file is byte-exact with mode 0600, and the link
// opens only once.
//
// Parameters:
//   - t: the test.
func TestAgentRoundTrip(t *testing.T) {
	for _, pass := range []bool{false, true} {
		for _, asJSON := range []bool{false, true} {
			te := newTestEnv(t)
			srv := newGoneServer(t, te)
			msgFile := writeTemp(t, "pw", "s3cret\n")
			sendArgs := []string{"send", "--json", "-s", srv.URL, "--message-file", msgFile}
			getArgs := []string{"get", "-"}
			if pass {
				pf := writeTemp(t, "pass", "correct horse battery\n")
				sendArgs = append(sendArgs, "--passphrase-file", pf)
				getArgs = append(getArgs, "--passphrase-file", pf)
			}
			link := runJSON(t, te, exitOK, sendArgs...)["link"]
			if asJSON {
				getArgs = append(getArgs, "--json")
			}
			dest := filepath.Join(t.TempDir(), "pw")
			te.reset()
			te.stdin.WriteString(link + "\n")
			if code := te.run(append(getArgs, "--message-out", dest)...); code != exitOK {
				t.Fatalf("get exit %d: %s", code, te.stderr)
			}
			checkSavedFile(t, dest, "s3cret\n")
			requireMessageFileReport(t, te, dest, asJSON)
			te.reset()
			te.stdin.WriteString(link)
			if code := te.run(append(getArgs, "--message-out", filepath.Join(t.TempDir(), "again"))...); code != exitNotFound {
				t.Fatalf("second get exit %d: %s", code, te.stderr)
			}
		}
	}
}

// requireMessageFileReport checks how a --message-out result is reported:
// JSON has message_file and no message key; text mode prints nothing on
// stdout and names the file on stderr.
//
// Parameters:
//   - t: the test.
//   - te: environment with captured output.
//   - dest: message file path.
//   - asJSON: whether --json was used.
func requireMessageFileReport(t *testing.T, te *testEnv, dest string, asJSON bool) {
	t.Helper()
	if strings.Contains(te.stdout.String()+te.stderr.String(), "s3cret") {
		t.Fatalf("plaintext in output: %q %q", te.stdout, te.stderr)
	}
	if !asJSON {
		if te.stdout.Len() != 0 || !strings.Contains(te.stderr.String(), "Saved message: "+dest+" (7 bytes)") {
			t.Fatalf("stdout %q stderr %q", te.stdout, te.stderr)
		}
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(te.stdout.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	var mf messageFile
	if _, ok := raw["message"]; ok || json.Unmarshal(raw["message_file"], &mf) != nil || mf.Path != dest || mf.Size != 7 {
		t.Fatalf("json = %s", te.stdout)
	}
}

// TestGetMessageOutBesideAttachment checks that an attachment with the
// same name as the message file gets a numbered name.
//
// Parameters:
//   - t: the test.
func TestGetMessageOutBesideAttachment(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	att := writeTemp(t, "a.txt", "body")
	link, _ := sendText(t, te, "msg", "-s", srv.URL, "-f", att)
	out := t.TempDir()
	te.reset()
	got := getJSON(t, te, link, "--json", "-o", out, "--message-out", filepath.Join(out, "a.txt"))
	if len(got.Files) != 1 || got.Files[0].Path != filepath.Join(out, "a (1).txt") {
		t.Fatalf("files = %+v", got.Files)
	}
	checkSavedFile(t, filepath.Join(out, "a.txt"), "msg")
	checkSavedFile(t, got.Files[0].Path, "body")
}

// TestGetEmptyMessageOut checks that an empty message still creates the
// file, so success always means the file exists.
//
// Parameters:
//   - t: the test.
func TestGetEmptyMessageOut(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link, _ := sendText(t, te, "", "-s", srv.URL, "-f", writeTemp(t, "a", "x"))
	dest := filepath.Join(t.TempDir(), "m")
	te.reset()
	if code := te.run("get", link, "-o", t.TempDir(), "--message-out", dest); code != exitOK {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	checkSavedFile(t, dest, "")
}

// countingServer starts the real server behind a wrapper that counts
// DELETE (acknowledge) requests and can fail them.
//
// Parameters:
//   - t: the test.
//   - te: environment that trusts the server.
//   - failAck: whether DELETE answers 500.
//
// Returns the server URL and the DELETE counter.
func countingServer(t *testing.T, te *testEnv, failAck bool) (string, *atomic.Int32) {
	t.Helper()
	inner := newGoneHandler(t)
	var acks atomic.Int32
	srv := newTLS(t, te, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			acks.Add(1)
			if failAck {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
		}
		inner.ServeHTTP(w, r)
	}))
	return srv.URL, &acks
}

// fillCollisions creates name and every numbered variant of it in dir, so
// saving an attachment called name fails.
//
// Parameters:
//   - t: the test.
//   - dir: output directory.
//   - name: attachment name.
func fillCollisions(t *testing.T, dir, name string) {
	t.Helper()
	for n := 0; n <= maxCollisionSuffix; n++ {
		if err := os.WriteFile(filepath.Join(dir, collisionName(name, n)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestGetMessageOutRollback checks that when an attachment cannot be
// saved after the message file was written, the message file is removed
// and the secret is not acknowledged.
//
// Parameters:
//   - t: the test.
func TestGetMessageOutRollback(t *testing.T) {
	te := newTestEnv(t)
	url, acks := countingServer(t, te, false)
	link, _ := sendText(t, te, "msg", "-s", url, "-f", writeTemp(t, "a.txt", "x"))
	out := t.TempDir()
	fillCollisions(t, out, "a.txt")
	dest := filepath.Join(t.TempDir(), "m")
	te.reset()
	if code := te.run("get", link, "-o", out, "--message-out", dest); code != exitIO {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) || acks.Load() != 0 {
		t.Fatalf("message file kept (%v) or acked (%d)", err, acks.Load())
	}
}

// TestGetMessageOutAckFailure checks that the message file is kept when
// the server does not confirm deletion.
//
// Parameters:
//   - t: the test.
func TestGetMessageOutAckFailure(t *testing.T) {
	te := newTestEnv(t)
	url, _ := countingServer(t, te, true)
	link, _ := sendText(t, te, "msg", "-s", url)
	dest := filepath.Join(t.TempDir(), "m")
	te.reset()
	if code := te.run("get", "--json", link, "--message-out", dest); code != exitNetwork {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	if !strings.Contains(te.stderr.String(), "not_confirmed") || strings.Contains(te.stdout.String(), "msg\"") {
		t.Fatalf("stdout %q stderr %q", te.stdout, te.stderr)
	}
	checkSavedFile(t, dest, "msg")
}

// TestManageLinkFromStdin checks status and revoke with "-".
//
// Parameters:
//   - t: the test.
func TestManageLinkFromStdin(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	_, manage := sendText(t, te, "hi", "-s", srv.URL)
	for _, cmd := range []string{"status", "revoke"} {
		te.reset()
		te.stdin.WriteString(manage + "\n")
		if code := te.run(cmd, "-"); code != exitOK {
			t.Fatalf("%s exit %d: %s", cmd, code, te.stderr)
		}
	}
	te.reset()
	te.Stdin = errReader{}
	if code := te.run("status", "-"); code != exitIO {
		t.Fatalf("read error exit %d: %s", code, te.stderr)
	}
}

// TestGetMessageOutRace checks a destination that appears after the
// preflight but before the write: the existing file is kept untouched,
// nothing else is written, and the secret is not acknowledged.
//
// Parameters:
//   - t: the test.
func TestGetMessageOutRace(t *testing.T) {
	te := newTestEnv(t)
	dest := filepath.Join(t.TempDir(), "m")
	inner := newGoneHandler(t)
	var acks atomic.Int32
	srv := newTLS(t, te, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = os.WriteFile(dest, []byte("planted"), 0o600)
		case http.MethodDelete:
			acks.Add(1)
		}
		inner.ServeHTTP(w, r)
	}))
	link, _ := sendText(t, te, "msg", "-s", srv.URL, "-f", writeTemp(t, "a.txt", "x"))
	out := t.TempDir()
	te.reset()
	if code := te.run("get", link, "-o", out, "--message-out", dest); code != exitIO {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	checkSavedFile(t, dest, "planted")
	if entries, _ := os.ReadDir(out); len(entries) != 0 || acks.Load() != 0 {
		t.Fatalf("files %v, acks %d", entries, acks.Load())
	}
}

// TestGetLinkReadError checks a failing stdin for "get -".
//
// Parameters:
//   - t: the test.
func TestGetLinkReadError(t *testing.T) {
	te := newTestEnv(t)
	te.Stdin = errReader{}
	if code := te.run("get", "-"); code != exitIO {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
}
