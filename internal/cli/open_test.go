package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/domain"
)

// TestRecipientPassphrase checks every version and source combination.
//
// Parameters:
//   - t: the test.
func TestRecipientPassphrase(t *testing.T) {
	file := writeTemp(t, "pass", "hunter22\n")
	tests := []struct {
		name    string
		version uint8
		file    string
		tty     bool
		want    string
		usage   bool
	}{
		{"v1 with file", domain.ProtocolV1, file, false, "", true},
		{"v1 plain", domain.ProtocolV1, "", false, "", false},
		{"v2 file", domain.ProtocolV2, file, false, "hunter22", false},
		{"v2 no tty", domain.ProtocolV2, "", false, "", true},
		{"v2 tty prompts later", domain.ProtocolV2, "", true, "", false},
	}
	for _, tt := range tests {
		te := newTestEnv(t)
		te.StdinTTY = tt.tty
		a := &app{env: te.Env}
		got, err := a.recipientPassphrase(tt.version, tt.file)
		var ue *usageError
		if errors.As(err, &ue) != tt.usage || string(got) != tt.want {
			t.Errorf("%s: got %q, err %v", tt.name, got, err)
		}
	}
}

// TestDecryptPromptError checks that a prompt failure stops decryption
// rather than counting as a wrong passphrase.
//
// Parameters:
//   - t: the test.
func TestDecryptPromptError(t *testing.T) {
	te := newTestEnv(t)
	te.StdinTTY = true
	a := &app{env: te.Env}
	s, err := sealPayloadForTest(t, "pw-123456")
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.decrypt(domain.ProtocolV2, s.key, nil, s.claimed())
	if err == nil || errors.Is(err, errWrongPassphrase) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(te.stderr.String(), "try again") {
		t.Fatalf("unexpected retry: %q", te.stderr)
	}
}

// TestInvalidEntryErrorMessage checks that the rejection reason is the
// error text, so the retry prompt can show it.
//
// Parameters:
//   - t: the test.
func TestInvalidEntryErrorMessage(t *testing.T) {
	if got := (&invalidEntryError{msg: "too long"}).Error(); got != "too long" {
		t.Fatalf("Error() = %q", got)
	}
}
