package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// getResult is the JSON document printed by "gone get --json".
type getResult struct {
	Message string      `json:"message"`
	Files   []savedFile `json:"files"`
}

// getJSON runs "gone get" with args, requires success and decodes the
// JSON result.
//
// Parameters:
//   - t: the test.
//   - te: test environment.
//   - args: arguments after "get".
//
// Returns the decoded result.
func getJSON(t *testing.T, te *testEnv, args ...string) getResult {
	t.Helper()
	if code := te.run(append([]string{"get"}, args...)...); code != exitOK {
		t.Fatalf("get exit %d: %s", code, te.stderr)
	}
	var got getResult
	if err := json.Unmarshal(te.stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	return got
}

// checkSendJSON checks that a send result has every field and a link of
// the expected protocol version.
//
// Parameters:
//   - t: the test.
//   - raw: decoded send result.
//   - version: fragment prefix such as "#v2:".
func checkSendJSON(t *testing.T, raw map[string]string, version string) {
	t.Helper()
	if !strings.Contains(raw["link"], version) || raw["manage_link"] == "" || raw["id"] == "" || raw["expires_at"] == "" {
		t.Fatalf("json result = %v", raw)
	}
}

// runJSON runs a command, checks its exit code and decodes its stdout as
// a JSON object of strings.
//
// Parameters:
//   - t: the test.
//   - te: test environment.
//   - want: expected exit code.
//   - args: command arguments.
//
// Returns the decoded object.
func runJSON(t *testing.T, te *testEnv, want int, args ...string) map[string]string {
	t.Helper()
	if code := te.run(args...); code != want {
		t.Fatalf("%s exit %d: %s", args[0], code, te.stderr)
	}
	var raw map[string]string
	if err := json.Unmarshal(te.stdout.Bytes(), &raw); err != nil {
		t.Fatalf("json: %v %q", err, te.stdout)
	}
	return raw
}

// checkSavedFile checks a saved attachment's contents and owner-only mode.
//
// Parameters:
//   - t: the test.
//   - path: saved file path.
//   - want: expected contents.
func checkSavedFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test temp dir
	if err != nil || string(data) != want {
		t.Fatalf("read %s: %q %v", path, data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("stat %s: %v %v", path, info, err)
	}
}
