package cli

import (
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// Valid links of each kind, checked by TestTestLinks.
var (
	testID          = strings.Repeat("0f", 16)
	testSecretLink  = "https://app.getgone.dev/secret/" + testID + "#v1:OwCB6YyKW8_dL2kTNRJXbOsx8GkCQ9MgSLt2iuzfL1w"
	testManageLink  = "https://app.getgone.dev/manage/" + testID + "#" + strings.Repeat("Q", 42) + "A"
	testRequestLink = "https://app.getgone.dev/reply/" + testID + "#v3:B" + strings.Repeat("A", 86) + "." + strings.Repeat("Q", 42) + "A"
)

// TestTestLinks checks that the links the tests below start from are
// valid.
//
// Parameters:
//   - t: the test handle.
func TestTestLinks(t *testing.T) {
	if _, err := envelope.ParseLink(testSecretLink); err != nil {
		t.Fatalf("secret link: %v", err)
	}
	if _, err := envelope.ParseManageLink(testManageLink); err != nil {
		t.Fatalf("manage link: %v", err)
	}
	if _, err := envelope.ParseReplyLink(testRequestLink); err != nil {
		t.Fatalf("request link: %v", err)
	}
}

// TestNormalizeLink checks that the ways shells, editors and mail clients
// mangle a pasted link are undone, and that a valid link of every kind is
// left exactly as it is.
//
// Parameters:
//   - t: the test handle.
func TestNormalizeLink(t *testing.T) {
	for _, link := range []string{testSecretLink, testManageLink, testRequestLink, "http://localhost:8080/secret/" + testID + "#v1:k"} {
		if got := normalizeLink(link); got != link {
			t.Errorf("valid link changed: %q -> %q", link, got)
		}
		half := len(link) / 2
		for name, in := range map[string]string{
			"zsh paste":         strings.Replace(link, "#", `\#`, 1),
			"zsh paste, quoted": `'` + strings.Replace(link, "#", `\#`, 1) + `'`,
			"cmd single quotes": `'` + link + `'`,
			"double quotes":     `"` + link + `"`,
			"smart single":      "‘" + link + "’",
			"smart double":      "“" + link + "”",
			"angle brackets":    "<" + link + ">",
			"parentheses":       "(" + link + ").",
			"end of sentence":   link + ".",
			"question":          link + "?",
			"surrounding space": " \t" + link + " \r\n",
			"wrapped line":      link[:half] + "\r\n" + link[half:],
			"no-break space":    "\u00a0" + link + "\u00a0",
			"zero-width space":  link[:half] + "\u200b" + link[half:],
			"byte-order mark":   "\xef\xbb\xbf" + link,
		} {
			if got := normalizeLink(in); got != link {
				t.Errorf("%s: %q -> %q, want %q", name, in, got, link)
			}
		}
	}
	if got := unescapePastedLink(`a\b`); got != `a\b` {
		t.Errorf("backslash before a letter: %q", got)
	}
	if got := unescapePastedLink(`end\`); got != `end\` {
		t.Errorf("trailing backslash: %q", got)
	}
}

// TestDiagnoseLink checks the reason given for each kind of bad link, and
// that no reason repeats any part of the link.
//
// Parameters:
//   - t: the test handle.
func TestDiagnoseLink(t *testing.T) {
	key := "OwCB6YyKW8_dL2kTNRJXbOsx8GkCQ9MgSLt2iuzfL1w"
	tests := []struct {
		name string
		link string
		want linkKind
		says string
	}{
		{"not a URL", "app.getgone.dev/secret/" + testID, secretLinkKind, "should start with https://"},
		{"scanner", "https://nam12.safelinks.protection.outlook.com/?url=https%3A%2F%2Fapp.getgone.dev%2Fsecret%2F" + testID + "%23v1%3A" + key, secretLinkKind, "link scanner"},
		{"proofpoint", "https://urldefense.com/v3/__https://app.getgone.dev/secret/" + testID, secretLinkKind, "link scanner"},
		{"encoded #", "https://app.getgone.dev/secret/" + testID + "%23v1:" + key, secretLinkKind, "%23"},
		{"manage link to get", testManageLink, secretLinkKind, "use gone status or gone revoke"},
		{"request link to get", testRequestLink, secretLinkKind, "use gone reply"},
		{"secret link to reply", testSecretLink, requestLinkKind, "use gone get"},
		{"secret link to status", testSecretLink, manageLinkKind, "use gone get"},
		{"other site", "https://example.com/nope", secretLinkKind, "isn't a link to a Gone secret"},
		{"newer version", "https://app.getgone.dev/secret/" + testID + "#v9:" + key, secretLinkKind, "newer version of Gone"},
		{"no key", "https://app.getgone.dev/secret/" + testID, secretLinkKind, "key (the part after #) is missing"},
		{"cut short", testSecretLink[:len(testSecretLink)-5], secretLinkKind, "incomplete or damaged"},
		{"request cut short", testRequestLink[:len(testRequestLink)-5], requestLinkKind, "incomplete or damaged"},
	}
	parsers := map[linkKind]func(string) error{
		secretLinkKind:  func(s string) error { _, err := envelope.ParseLink(s); return err },
		manageLinkKind:  func(s string) error { _, err := envelope.ParseManageLink(s); return err },
		requestLinkKind: func(s string) error { _, err := envelope.ParseReplyLink(s); return err },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parsers[tt.want](tt.link)
			if err == nil {
				t.Fatal("the link parsed")
			}
			got := diagnoseLink(tt.link, tt.want, err)
			if !strings.HasPrefix(got, "not a valid "+linkKinds[tt.want].noun+": ") || !strings.Contains(got, tt.says) {
				t.Fatalf("diagnoseLink = %q, want %q", got, tt.says)
			}
			for _, part := range []string{testID, key[:8], "app.getgone.dev"} {
				if strings.Contains(got, part) {
					t.Fatalf("message repeats %q: %q", part, got)
				}
			}
		})
	}
}

// TestEOFKey checks the end-of-input key named in prompts.
//
// Parameters:
//   - t: the test handle.
func TestEOFKey(t *testing.T) {
	if got := eofKeyFor("windows"); got != "Ctrl-Z then Enter" {
		t.Errorf("windows: %q", got)
	}
	for _, goos := range []string{"linux", "darwin", "freebsd"} {
		if got := eofKeyFor(goos); got != "Ctrl-D" {
			t.Errorf("%s: %q", goos, got)
		}
	}
	if !strings.Contains(messagePrompt, eofKey) || !strings.Contains(linkPrompt, eofKey) || !strings.Contains(usageSend, eofKey) {
		t.Error("a prompt doesn't name the key")
	}
}

// TestGitBashHint checks the hint is given only in an MSYS terminal on
// Windows.
//
// Parameters:
//   - t: the test handle.
func TestGitBashHint(t *testing.T) {
	if !strings.Contains(gitBashHint("windows", "MINGW64"), "winpty") {
		t.Error("no hint in Git Bash")
	}
	if gitBashHint("windows", "") != "" || gitBashHint("linux", "MINGW64") != "" {
		t.Error("hint outside Git Bash")
	}
}
