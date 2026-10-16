package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runBridge sends one request through the browser code via the interop
// script (test/interop/interop.js) and decodes its reply into out.
//
// Parameters:
//   - t: the test.
//   - req: the request; marshaled as JSON onto the script's stdin.
//   - out: where the JSON reply is decoded on success.
//
// Returns "" on success, or the error code the browser code threw (the last
// line the script wrote to stderr) when it exited with a failure.
func runBridge(t *testing.T, req, out any) string {
	t.Helper()
	in, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// #nosec G204 -- fixed script path; node resolved from PATH by the test.
	cmd := exec.CommandContext(ctx, requireNode(t), filepath.Join("interop", "interop.js"))
	cmd.Stdin = bytes.NewReader(in)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && ctx.Err() == nil {
		return lastLine(stderr.String())
	}
	if err != nil {
		t.Fatalf("interop: %v: %s", err, stderr.String())
	}
	if err := json.Unmarshal(stdout, out); err != nil {
		t.Fatalf("interop reply %q: %v", stdout, err)
	}
	return ""
}

// lastLine returns the last non-empty line of s, or "?" when there is none.
//
// Parameters:
//   - s: text such as a process's stderr.
//
// Returns the line without surrounding whitespace.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return "?"
}
