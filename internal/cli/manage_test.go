package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestManageErrors checks status and revoke argument and link errors.
//
// Parameters:
//   - t: the test.
func TestManageErrors(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	_, manage := sendText(t, te, "hi", "--server", srv.URL)
	httpManage := "http" + strings.TrimPrefix(manage, "https")
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"status no link", []string{"status"}, exitUsage, "manage link"},
		{"revoke two links", []string{"revoke", manage, manage}, exitUsage, ""},
		{"bad timeout", []string{"status", "--timeout", "-1s", manage}, exitUsage, "timeout"},
		{"bad link", []string{"status", srv.URL + "/secret/x#v1:y"}, exitUsage, "not a valid manage link"},
		{"http link", []string{"revoke", httpManage}, exitUsage, "--insecure"},
	}
	for _, tt := range tests {
		te.reset()
		if code := te.run(tt.args...); code != tt.code || !strings.Contains(te.stderr.String(), tt.want) {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, te.stderr)
		}
	}
}

// TestManageServerErrors checks that status and revoke surface server
// failures.
//
// Parameters:
//   - t: the test.
func TestManageServerErrors(t *testing.T) {
	te := newTestEnv(t)
	inner := newGoneHandler(t)
	fail := false
	srv := newTLS(t, te, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	_, manage := sendText(t, te, "hi", "--server", srv.URL)
	fail = true
	for _, cmd := range []string{"status", "revoke"} {
		te.reset()
		if code := te.run(cmd, manage); code != exitNetwork || te.stdout.Len() != 0 {
			t.Errorf("%s: exit %d, stdout %q", cmd, code, te.stdout)
		}
	}
}

// TestStatusPending checks text and JSON status of a pending secret.
//
// Parameters:
//   - t: the test.
func TestStatusPending(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	_, manage := sendText(t, te, "a", "-s", srv.URL)

	te.reset()
	if code := te.run("status", manage); code != exitOK {
		t.Fatalf("status exit %d: %s", code, te.stderr)
	}
	if linkLine(t, te.stdout.String(), "State:") != "pending" {
		t.Fatalf("status %q", te.stdout)
	}

	te.reset()
	st := runJSON(t, te, exitOK, "status", "--json", manage)
	if st["state"] != "pending" || st["created_at"] == "" {
		t.Fatalf("status json %v", st)
	}
}

// TestRevokeRemovesSecret checks that revoke succeeds once, then reports
// not found, and that the revoked secret cannot be retrieved.
//
// Parameters:
//   - t: the test.
func TestRevokeRemovesSecret(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	link, manage := sendText(t, te, "a", "-s", srv.URL)

	te.reset()
	if code := te.run("revoke", manage); code != exitOK || te.stdout.String() != "Revoked.\n" {
		t.Fatalf("revoke exit %d: %s %q", code, te.stderr, te.stdout)
	}
	te.reset()
	if code := te.run("revoke", manage, "--json"); code != exitNotFound {
		t.Fatalf("second revoke exit %d", code)
	}
	var e map[string]string
	if err := json.Unmarshal(te.stderr.Bytes(), &e); err != nil || e["error"] != "not_found" {
		t.Fatalf("json error %q", te.stderr)
	}
	te.reset()
	if code := te.run("get", link); code != exitNotFound {
		t.Fatalf("get after revoke exit %d", code)
	}
}

// TestRevokeJSON checks the JSON output of revoke.
//
// Parameters:
//   - t: the test.
func TestRevokeJSON(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	_, manage := sendText(t, te, "a", "-s", srv.URL)
	te.reset()
	if code := te.run("revoke", "--json", manage); code != exitOK || te.stdout.String() != "{\"revoked\":true}\n" {
		t.Fatalf("exit %d: %q %s", code, te.stdout, te.stderr)
	}
}
