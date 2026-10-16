package cli

import (
	"strings"
	"testing"
)

// foreignOrigin is a server the leak server is not.
const foreignOrigin = "https://elsewhere.example"

// foreignLink points link at foreignOrigin, keeping its path and fragment.
//
// Parameters:
//   - l: environment whose server issued the link.
//   - link: link on the leak server.
//
// Returns the rewritten link.
func (l *leakEnv) foreignLink(link string) string {
	return foreignOrigin + strings.TrimPrefix(link, l.url)
}

// originCase builds a case where cmd gets a link for a server other than
// the configured one and no --allow-origin, so it must refuse before any
// request and without printing the link's ID, fragment, key or tokens.
//
// Parameters:
//   - name: case name.
//   - cmd: command that takes a link on stdin.
//   - pick: chooses the link to rewrite: share, manage or request.
//
// Returns the case.
func originCase(name, cmd, pick string) leakCase {
	return leakCase{name, func(l *leakEnv) (string, []string, int) {
		var link string
		switch pick {
		case "request":
			link, _ = l.request(l.url)
		case "manage":
			_, link = l.send(l.url)
		default:
			link, _ = l.send(l.url)
		}
		l.te.strict = true
		args := []string{cmd, "-", "-s", l.url}
		if cmd == "reply" {
			args = append(args, "--message-file", l.msgFile, "-f", l.attFile)
		}
		return l.foreignLink(link), args, exitUsage
	}}
}

// originLeakCases cover the origin policy refusal of every link command.
var originLeakCases = []leakCase{
	originCase("get untrusted origin", "get", "share"),
	originCase("status untrusted origin", "status", "manage"),
	originCase("revoke untrusted origin", "revoke", "manage"),
	originCase("reply untrusted origin", "reply", "request"),
}

// TestOriginLeakCasesRefuse checks the origin cases really end in the
// untrusted-origin refusal, naming both origins, so the leak checks above
// exercise that path and not some other usage error.
//
// Parameters:
//   - t: the test.
func TestOriginLeakCasesRefuse(t *testing.T) {
	for _, tc := range originLeakCases {
		l := newLeakEnv(t)
		stdin, args, _ := tc.setup(l)
		l.te.reset()
		l.te.stdin.WriteString(stdin)
		args = append(args, "--json")
		if code := l.te.run(args...); code != exitUsage {
			t.Fatalf("%s: exit %d", tc.name, code)
		}
		out := l.te.stdout.String() + l.te.stderr.String()
		for _, want := range []string{"untrusted_origin", foreignOrigin, l.url} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: %q missing from %q", tc.name, want, out)
			}
		}
	}
}
