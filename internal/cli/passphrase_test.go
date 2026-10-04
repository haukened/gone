package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestSendPassphrase checks each passphrase source and their conflicts.
//
// Parameters:
//   - t: the test handle.
func TestSendPassphrase(t *testing.T) {
	good := writeTemp(t, "p", "longenough\r\n")
	short := writeTemp(t, "s", "short\n")
	tests := []struct {
		name      string
		opts      passOpts
		tty       bool
		passwords []string
		want      string
		code      int
	}{
		{"none", passOpts{}, false, nil, "", exitOK},
		{"conflict", passOpts{prompt: true, generate: true}, false, nil, "", exitUsage},
		{"file", passOpts{file: good}, false, nil, "longenough", exitOK},
		{"file short", passOpts{file: short}, false, nil, "", exitUsage},
		{"file missing", passOpts{file: filepath.Join(t.TempDir(), "x")}, false, nil, "", exitIO},
		{"prompt", passOpts{prompt: true}, true, []string{"longenough", "longenough"}, "longenough", exitOK},
		{"prompt no tty", passOpts{prompt: true}, false, nil, "", exitUsage},
		{"prompt short", passOpts{prompt: true}, true, []string{"short"}, "", exitUsage},
		{"prompt mismatch", passOpts{prompt: true}, true, []string{"longenough", "different!"}, "", exitUsage},
		{"prompt confirm fails", passOpts{prompt: true}, true, []string{"longenough"}, "", exitIO},
		{"prompt empty", passOpts{prompt: true}, true, []string{""}, "", exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			te.StdinTTY = tt.tty
			te.passwords = tt.passwords
			a := &app{env: te.Env}
			p, gen, err := a.sendPassphrase(tt.opts)
			if err != nil {
				code := classify(err).code
				if code != tt.code {
					t.Fatalf("err = %v (code %d)", err, code)
				}
				return
			}
			if tt.code != exitOK || string(p) != tt.want || gen {
				t.Fatalf("got %q gen=%v err=%v", p, gen, err)
			}
		})
	}
}

// TestSendPassphraseGenerate checks a generated passphrase is flagged.
//
// Parameters:
//   - t: the test handle.
func TestSendPassphraseGenerate(t *testing.T) {
	a := &app{env: newTestEnv(t).Env}
	p, gen, err := a.sendPassphrase(passOpts{generate: true})
	if err != nil || !gen || len(p) < minPassphraseRunes {
		t.Fatalf("got %q gen=%v err=%v", p, gen, err)
	}
}

// TestReadPassphraseFile checks size, encoding and read errors.
//
// Parameters:
//   - t: the test handle.
func TestReadPassphraseFile(t *testing.T) {
	big := writeTemp(t, "big", strings.Repeat("a", 1025))
	if _, err := readPassphraseFile(big); classify(err).code != exitUsage {
		t.Fatalf("big err = %v", err)
	}
	bad := writeTemp(t, "bad", "\xff\xfe")
	if _, err := readPassphraseFile(bad); classify(err).code != exitUsage {
		t.Fatalf("invalid err = %v", err)
	}
	if _, err := readPassphraseFile(t.TempDir()); classify(err).code != exitIO {
		t.Fatalf("dir err = %v", err)
	}
	maxPath := writeTemp(t, "max", strings.Repeat("a", 1024)+"\r\n")
	if p, err := readPassphraseFile(maxPath); err != nil || len(p) != 1024 {
		t.Fatalf("max = %d, %v", len(p), err)
	}
}

// TestTrimNewline checks only one trailing line ending is removed.
//
// Parameters:
//   - t: the test handle.
func TestTrimNewline(t *testing.T) {
	tests := map[string]string{
		"":        "",
		"\n":      "",
		"\r\n":    "",
		"a\n\n":   "a\n",
		"a\r":     "a\r",
		"abc":     "abc",
		"a\r\r\n": "a\r",
	}
	for in, want := range tests {
		if got := string(trimNewline([]byte(in))); got != want {
			t.Errorf("trimNewline(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPromptPasswordReadError checks a terminal read failure.
//
// Parameters:
//   - t: the test handle.
func TestPromptPasswordReadError(t *testing.T) {
	te := newTestEnv(t)
	te.StdinTTY = true
	te.ReadPassword = func() ([]byte, error) { return nil, errors.New("tty gone") }
	a := &app{env: te.Env}
	if _, err := a.promptPassword("P: "); classify(err).code != exitIO {
		t.Fatalf("err = %v", err)
	}
	if !strings.HasPrefix(te.stderr.String(), "P: ") {
		t.Fatalf("stderr = %q", te.stderr)
	}
}
