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
	p, err := a.readPayload(inputOpts{files: stringList{f}})
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
		files    stringList
		badStdin bool
		want     int
	}{
		{"empty", nil, false, exitUsage},
		{"too many", many, false, exitUsage},
		{"directory", stringList{t.TempDir()}, false, exitUsage},
		{"missing", stringList{f, filepath.Join(t.TempDir(), "x")}, false, exitIO},
		{"stdin error", stringList{f}, true, exitIO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			if tt.badStdin {
				te.Stdin = errReader{}
			}
			a := &app{env: te.Env}
			if _, err := a.readPayload(inputOpts{files: tt.files}); classify(err).code != tt.want {
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
	if _, err := a.readMessage(5, inputOpts{}); classify(err).code != exitUsage {
		t.Fatalf("message err = %v", err)
	}
	if !strings.Contains(te.stderr.String(), eofKey) {
		t.Fatalf("no prompt: %q", te.stderr)
	}
	if !strings.Contains(tooLargeInput().Error(), "64 MiB") {
		t.Fatal("limit message")
	}
}

// TestReadPayloadMessageFile checks that --message-file keeps its trailing
// newline, shares the input budget, and never reads stdin or prompts.
//
// Parameters:
//   - t: the test handle.
func TestReadPayloadMessageFile(t *testing.T) {
	te := newTestEnv(t)
	te.Stdin = errReader{}
	te.StdinTTY = true
	a := &app{env: te.Env}
	msg := writeTemp(t, "m", "secret\n")
	p, err := a.readPayload(inputOpts{messageFile: msg})
	if err != nil || string(p.Message) != "secret\n" || te.stderr.Len() != 0 {
		t.Fatalf("payload = %q, %v, stderr %q", p.Message, err, te.stderr)
	}
	att := writeTemp(t, "a.bin", "12345")
	if _, err := a.readMessage(3, inputOpts{messageFile: msg}); classify(err).code != exitUsage {
		t.Fatalf("over budget: %v", err)
	}
	tests := []struct {
		name  string
		files stringList
		path  string
		want  int
	}{
		{"missing", nil, filepath.Join(t.TempDir(), "x"), exitIO},
		{"directory", nil, t.TempDir(), exitUsage},
		{"empty", nil, writeTemp(t, "e", ""), exitUsage},
		{"with attachment", stringList{att}, msg, exitOK},
	}
	for _, tt := range tests {
		_, err := a.readPayload(inputOpts{files: tt.files, messageFile: tt.path})
		got := exitOK
		if err != nil {
			got = classify(err).code
		}
		if got != tt.want {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
	if _, err := readRegularFile(filepath.Join(t.TempDir(), "x"), 1, "attachment"); !strings.Contains(err.Error(), "open attachment") {
		t.Fatalf("op text: %v", err)
	}
}

// TestReadLinkArg checks reading a "-" link from stdin.
//
// Parameters:
//   - t: the test handle.
func TestReadLinkArg(t *testing.T) {
	te := newTestEnv(t)
	a := &app{env: te.Env}
	if got, err := a.readLinkArg("https://x"); got != "https://x" || err != nil {
		t.Fatalf("passthrough = %q, %v", got, err)
	}
	te.stdin.WriteString("  https://x/secret/y#v1:k \n")
	if got, err := a.readLinkArg("-"); got != "https://x/secret/y#v1:k" || err != nil || te.stderr.Len() != 0 {
		t.Fatalf("stdin = %q, %v, stderr %q", got, err, te.stderr)
	}
	te.reset()
	te.StdinTTY = true
	te.stdin.WriteString(strings.Repeat("k", maxLinkInput+1))
	if _, err := a.readLinkArg("-"); classify(err).code != exitUsage || strings.Contains(err.Error(), "kkkk") {
		t.Fatalf("too long: %v", err)
	}
	if !strings.Contains(te.stderr.String(), "Paste the link") {
		t.Fatalf("no prompt: %q", te.stderr)
	}
	te.Stdin = errReader{}
	if _, err := a.readLinkArg("-"); classify(err).code != exitIO {
		t.Fatalf("read error: %v", err)
	}
}

// TestReadMessageText checks that a message from standard input loses one
// trailing newline unless --keep-newline is set, that --message-file keeps
// it, and that text with a byte-order mark becomes plain UTF-8 with a
// notice that holds none of the message.
//
// Parameters:
//   - t: the test handle.
func TestReadMessageText(t *testing.T) {
	utf16le := []byte{0xFF, 0xFE, 'h', 0, 'i', 0, '\r', 0, '\n', 0}
	tests := []struct {
		name   string
		stdin  string
		file   string
		keep   bool
		want   string
		notice string
	}{
		{"lf", "hunter2\n", "", false, "hunter2", ""},
		{"crlf", "hunter2\r\n", "", false, "hunter2", ""},
		{"only one", "a\n\n", "", false, "a\n", ""},
		{"none", "hunter2", "", false, "hunter2", ""},
		{"keep", "hunter2\n", "", true, "hunter2\n", ""},
		{"utf-8 bom", "\xEF\xBB\xBFhunter2\n", "", false, "hunter2", "byte-order mark"},
		{"utf-16 stdin", string(utf16le), "", false, "hi", "UTF-16"},
		{"file keeps newline", "", "hunter2\n", false, "hunter2\n", ""},
		{"utf-16 file", "", string(utf16le), false, "hi\r\n", "UTF-16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := newTestEnv(t)
			te.stdin.WriteString(tt.stdin)
			in := inputOpts{keepNewline: tt.keep}
			if tt.file != "" {
				in.messageFile = writeTemp(t, "m", tt.file)
			}
			a := &app{env: te.Env}
			got, err := a.readMessage(1<<20, in)
			if err != nil || string(got) != tt.want {
				t.Fatalf("message = %q, %v; want %q", got, err, tt.want)
			}
			if (tt.notice == "" && te.stderr.Len() != 0) || !strings.Contains(te.stderr.String(), tt.notice) {
				t.Fatalf("stderr = %q, want %q", te.stderr, tt.notice)
			}
			if strings.Contains(te.stderr.String(), "hunter2") {
				t.Fatalf("notice repeats the message: %q", te.stderr)
			}
		})
	}
}
