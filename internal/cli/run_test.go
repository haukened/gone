package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/client"
)

// TestRunHelpAndVersion checks the help, per-command help and version
// paths.
//
// Parameters:
//   - t: the test handle.
func TestRunHelpAndVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, usageMain},
		{"help", []string{"help"}, usageMain},
		{"-h", []string{"-h"}, usageMain},
		{"--help", []string{"--help"}, usageMain},
		{"help send", []string{"help", "send"}, usages["send"]},
		{"send -h", []string{"send", "-h"}, usages["send"]},
		{"get --help", []string{"get", "--help"}, usages["get"]},
		{"status -h", []string{"status", "-h"}, usages["status"]},
		{"revoke -h", []string{"revoke", "-h"}, usages["revoke"]},
		{"set -h", []string{"set", "-h"}, usages["set"]},
		{"version", []string{"version"}, "gone test\n"},
		{"--version", []string{"--version"}, "gone test\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			if code := te.run(tt.args...); code != exitOK {
				t.Fatalf("exit %d: %s", code, te.stderr)
			}
			if te.stdout.String() != tt.want {
				t.Fatalf("stdout = %q, want %q", te.stdout, tt.want)
			}
		})
	}
}

// TestRunUnknownCommand checks unknown commands and help topics in text
// and JSON modes.
//
// Parameters:
//   - t: the test handle.
func TestRunUnknownCommand(t *testing.T) {
	te := newTestEnv(t)
	if code := te.run("frobnicate"); code != exitUsage {
		t.Fatalf("exit %d", code)
	}
	requireUnknownCommandText(t, te.stderr.String())
	te.reset()
	if code := te.run("help", "nope"); code != exitUsage {
		t.Fatalf("help nope exit %d", code)
	}
	te.reset()
	if code := te.run("nope", "--json"); code != exitUsage {
		t.Fatalf("json exit %d", code)
	}
	var out map[string]any
	if err := json.Unmarshal(te.stderr.Bytes(), &out); err != nil {
		t.Fatalf("stderr not JSON: %q", te.stderr)
	}
	if out["error"] != "usage" {
		t.Fatalf("error = %v", out["error"])
	}
}

// requireUnknownCommandText verifies text-mode unknown command diagnostics.
//
// Parameters:
//   - t: the test handle.
//   - stderr: captured standard error text.
func requireUnknownCommandText(t *testing.T, stderr string) {
	t.Helper()
	for _, want := range []string{`unknown command "frobnicate"`, "gone help"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q", stderr)
		}
	}
}

// TestFinishJSONRetryAfter checks retry_after is rounded up to whole
// seconds and the terminal output is escaped.
//
// Parameters:
//   - t: the test handle.
func TestFinishJSONRetryAfter(t *testing.T) {
	te := newTestEnv(t)
	a := &app{env: te.Env, json: true}
	code := a.finish(&client.RateLimitError{RetryAfter: 1500 * time.Millisecond})
	if code != exitRateLimited {
		t.Fatalf("exit %d", code)
	}
	var out struct {
		Error      string `json:"error"`
		RetryAfter int64  `json:"retry_after"`
	}
	if err := json.Unmarshal(te.stderr.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error != "rate_limited" || out.RetryAfter != 2 {
		t.Fatalf("out = %+v", out)
	}
	te.reset()
	a.json = false
	a.finish(usagef("bad \x1b[31m"))
	if strings.Contains(te.stderr.String(), "\x1b") {
		t.Fatalf("escape not neutralised: %q", te.stderr)
	}
}

// TestWantsJSON checks detection of the --json flag before "--".
//
// Parameters:
//   - t: the test handle.
func TestWantsJSON(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"send", "--json"}, true},
		{[]string{"send", "-json"}, true},
		{[]string{"send", "--json=true"}, true},
		{[]string{"send", "--json=false"}, false},
		{[]string{"send", "--", "--json"}, false},
	}
	for _, tt := range tests {
		if got := wantsJSON(tt.args); got != tt.want {
			t.Errorf("wantsJSON(%q) = %v", tt.args, got)
		}
	}
}

// TestRunVersionWriteError checks a failed stdout write is an I/O error.
//
// Parameters:
//   - t: the test handle.
func TestRunVersionWriteError(t *testing.T) {
	te := newTestEnv(t)
	te.Stdout = errWriter{}
	if code := te.run("version"); code != exitIO {
		t.Fatalf("exit %d", code)
	}
}

// TestOSEnv checks OSEnv wires the process streams and version.
//
// Parameters:
//   - t: the test handle.
func TestOSEnv(t *testing.T) {
	env := OSEnv("1.2.3")
	checks := map[string]bool{
		"version":       env.Version == "1.2.3",
		"stdin":         env.Stdin != nil,
		"stdout":        env.Stdout != nil,
		"stderr":        env.Stderr != nil,
		"getenv":        env.Getenv != nil,
		"config_dir":    env.ConfigDir != nil,
		"read_password": env.ReadPassword != nil,
	}
	for name, ok := range checks {
		if !ok {
			t.Fatalf("%s missing in env = %+v", name, env)
		}
	}
}
