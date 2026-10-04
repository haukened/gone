package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestReadPayload checks attachment and message reading limits.
//
// Parameters:
//   - t: the test handle.
func TestReadPayload(t *testing.T) {
	te := newTestEnv(t)
	a := &app{env: te.Env}
	f := writeTemp(t, "a.txt", "hello")
	te.stdin.WriteString("msg")
	p, err := a.readPayload([]string{f})
	if err != nil || string(p.Message) != "msg" || len(p.Files) != 1 || p.Files[0].Name != "a.txt" {
		t.Fatalf("payload = %+v, %v", p, err)
	}
}

// TestReadPayloadErrors checks how payload input failures are classified.
//
// Parameters:
//   - t: the test handle.
func TestReadPayloadErrors(t *testing.T) {
	f := writeTemp(t, "a.txt", "hello")
	many := make([]string, 101)
	for i := range many {
		many[i] = f
	}
	tests := []struct {
		name     string
		files    []string
		badStdin bool
		want     int
	}{
		{"empty", nil, false, exitUsage},
		{"too many", many, false, exitUsage},
		{"directory", []string{t.TempDir()}, false, exitUsage},
		{"missing", []string{f, filepath.Join(t.TempDir(), "x")}, false, exitIO},
		{"stdin error", []string{f}, true, exitIO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			if tt.badStdin {
				te.Stdin = errReader{}
			}
			a := &app{env: te.Env}
			if _, err := a.readPayload(tt.files); classify(err).code != tt.want {
				t.Fatalf("err = %v, want code %d", err, tt.want)
			}
		})
	}
}

// TestReadLimits checks the shared input budget.
//
// Parameters:
//   - t: the test handle.
func TestReadLimits(t *testing.T) {
	f := writeTemp(t, "a.bin", "123456")
	if _, err := readAttachment(f, 5); classify(err).code != exitUsage {
		t.Fatalf("attachment err = %v", err)
	}
	if got, err := readAttachment(f, 6); err != nil || len(got.Data) != 6 {
		t.Fatalf("attachment = %+v, %v", got, err)
	}
	te := newTestEnv(t)
	te.StdinTTY = true
	te.stdin.WriteString("123456")
	a := &app{env: te.Env}
	if _, err := a.readMessage(5); classify(err).code != exitUsage {
		t.Fatalf("message err = %v", err)
	}
	if !strings.Contains(te.stderr.String(), "Ctrl-D") {
		t.Fatalf("no prompt: %q", te.stderr)
	}
	if !strings.Contains(tooLargeInput().Error(), "64 MiB") {
		t.Fatal("limit message")
	}
}
