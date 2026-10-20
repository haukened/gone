package integration_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Every suppression of a linter, scanner or vulnerability finding states its
// reason where it is written: on the same line, or in a comment on the line
// directly above. Vulnerability ignores also carry an expiry, so each one is
// rechecked. This test keeps that true; CONTRIBUTING.md states the rule.

// skipDirs are never scanned: generated output and third-party code.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "_site": true, "bin": true, ".tmp": true,
}

// directive matches one kind of suppression and returns the text after it
// that could be a reason.
type directive struct {
	name string
	re   *regexp.Regexp // group 1: whatever follows the directive itself
}

var directives = []directive{
	// #nosec G304,G306 -- reason / #nosec G304: reason / #nosec G304 reason
	{"#nosec", regexp.MustCompile(`#nosec(?:\s+G\d+(?:[,\s]+G\d+)*)?(.*)$`)},
	// //nolint:bodyclose // reason
	{"//nolint", regexp.MustCompile(`//nolint:\S+(.*)$`)},
	// # shellcheck disable=SC2329 # reason
	{"shellcheck disable", regexp.MustCompile(`shellcheck disable=\S+(.*)$`)},
	// # zizmor: ignore[cache-poisoning] reason
	{"zizmor: ignore", regexp.MustCompile(`zizmor: ignore\[[^\]]*\](.*)$`)},
	// // eslint-disable-next-line rule -- reason
	{"eslint-disable", regexp.MustCompile(`eslint-disable(?:-next-line|-line)?(?:\s+[\w/@,-]+)?(.*)$`)},
	// # pylint: disable=rule  # reason
	{"pylint: disable", regexp.MustCompile(`pylint: disable=\S+(.*)$`)},
}

// separators may sit between a directive and its reason, and nosemgrep
// rides along with #nosec; none of them is a reason.
var separators = regexp.MustCompile(`^(?:\s|--|//|#|:|nosemgrep)+`)

// hasReason reports whether the text after a directive explains it.
func hasReason(rest string) bool {
	rest = separators.ReplaceAllString(rest, "")
	rest = strings.TrimSpace(strings.TrimRight(rest, "*/ "))
	return len(strings.Fields(rest)) >= 2
}

// commentAbove reports whether line is a comment with words in it.
func commentAbove(line string) bool {
	t := strings.TrimSpace(line)
	for _, p := range []string{"//", "#", "/*", "*"} {
		if strings.HasPrefix(t, p) {
			return hasReason(strings.TrimPrefix(t, p))
		}
	}
	return false
}

func scannable(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".sh", ".py", ".js", ".cjs", ".yaml", ".yml":
		return true
	}
	return false
}

func TestSuppressionsStateTheirReason(t *testing.T) {
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if scannable(path) && !strings.HasSuffix(path, "suppressions_test.go") {
			checkFile(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path) // #nosec G304 -- walking this repository's own files
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	prev := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		for _, d := range directives {
			m := d.re.FindStringSubmatch(line)
			if m != nil && !hasReason(m[1]) && !commentAbove(prev) {
				t.Errorf("%s:%d: %s without a reason; say why on the same line or in a comment just above", path, n, d.name)
			}
		}
		prev = line
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestCodacyExclusionsStateTheirReason checks every path .codacy.yml keeps
// out of analysis has a comment saying why.
func TestCodacyExclusionsStateTheirReason(t *testing.T) {
	raw, err := os.ReadFile("../.codacy.yml")
	if err != nil {
		t.Fatal(err)
	}
	for n, line := range strings.Split(string(raw), "\n") {
		t2 := strings.TrimSpace(line)
		if !strings.HasPrefix(t2, "- ") {
			continue
		}
		_, comment, ok := strings.Cut(t2, " # ")
		if !ok || !hasReason(comment) {
			t.Errorf(".codacy.yml:%d: excluded path without a reason comment: %s", n+1, t2)
		}
	}
}

// TestVulnerabilityIgnoresExpire checks each OSV-Scanner ignore has a reason
// and an ignoreUntil date at most a year away, so it is rechecked.
func TestVulnerabilityIgnoresExpire(t *testing.T) {
	files, err := filepath.Glob("../tools/*/osv-scanner.toml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no osv-scanner.toml files found: %v", err)
	}
	limit := time.Now().AddDate(1, 0, 0)
	until := regexp.MustCompile(`(?m)^ignoreUntil\s*=\s*(\d{4}-\d{2}-\d{2})`)
	reason := regexp.MustCompile(`(?m)^reason\s*=\s*"[^"]{20,}"`)
	for _, path := range files {
		raw, err := os.ReadFile(path) // #nosec G304 -- this repository's own config files
		if err != nil {
			t.Fatal(err)
		}
		for i, block := range strings.Split(string(raw), "[[IgnoredVulns]]")[1:] {
			if !reason.MatchString(block) {
				t.Errorf("%s: ignore %d has no reason", path, i+1)
			}
			m := until.FindStringSubmatch(block)
			if m == nil {
				t.Errorf("%s: ignore %d has no ignoreUntil date; give it one at most a year away", path, i+1)
				continue
			}
			day, err := time.Parse(time.DateOnly, m[1])
			if err != nil || day.After(limit) {
				t.Errorf("%s: ignore %d expires %s, more than a year away; recheck it sooner", path, i+1, m[1])
			}
		}
	}
}
