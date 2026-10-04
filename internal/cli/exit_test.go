package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/client"
	"github.com/haukened/gone/internal/envelope"
)

// TestClassify checks every error maps to the documented exit code and
// kind.
//
// Parameters:
//   - t: the test handle.
func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		kind string
	}{
		{"ack wraps not found", &ackError{err: client.ErrNotFound}, exitNetwork, "not_confirmed"},
		{"usage", usagef("bad %s", "x"), exitUsage, "usage"},
		{"rate limited", fmt.Errorf("w: %w", &client.RateLimitError{RetryAfter: time.Second}), exitRateLimited, "rate_limited"},
		{"io", ioErr("read", errors.New("boom")), exitIO, "io"},
		{"invalid passphrase", envelope.ErrInvalidPassphrase, exitUsage, "usage"},
		{"invalid origin", client.ErrInvalidOrigin, exitUsage, "usage"},
		{"insecure origin", client.ErrInsecureOrigin, exitUsage, "usage"},
		{"not found", client.ErrNotFound, exitNotFound, "not_found"},
		{"wrong passphrase", errWrongPassphrase, exitPassphrase, "wrong_passphrase"},
		{"integrity", client.ErrIntegrity, exitIntegrity, "integrity"},
		{"version mismatch", client.ErrVersionMismatch, exitIntegrity, "integrity"},
		{"decrypt", envelope.ErrDecrypt, exitIntegrity, "integrity"},
		{"malformed", envelope.ErrMalformed, exitIntegrity, "integrity"},
		{"invalid envelope", envelope.ErrInvalidEnvelope, exitIntegrity, "integrity"},
		{"too many files", envelope.ErrTooManyFiles, exitIntegrity, "integrity"},
		{"network", client.ErrNetwork, exitNetwork, "network"},
		{"too large", client.ErrTooLarge, exitNetwork, "too_large"},
		{"server", client.ErrServer, exitNetwork, "server"},
		{"rejected", client.ErrRejected, exitNetwork, "server"},
		{"protocol", client.ErrProtocol, exitNetwork, "server"},
		{"canceled", context.Canceled, exitInternal, "interrupted"},
		{"other", errors.New("mystery"), exitInternal, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := classify(tt.err)
			if f.code != tt.code || f.kind != tt.kind {
				t.Fatalf("classify = %+v, want code %d kind %s", f, tt.code, tt.kind)
			}
			if f.msg == "" {
				t.Fatal("empty message")
			}
		})
	}
}

// TestClassifyMessages checks the message details carried by the error
// wrappers.
//
// Parameters:
//   - t: the test handle.
func TestClassifyMessages(t *testing.T) {
	if f := classify(ioErr("read", errors.New("boom"))); f.msg != "read: boom" {
		t.Fatalf("io msg = %q", f.msg)
	}
	if f := classify(fmt.Errorf("dial: %w", client.ErrNetwork)); !strings.Contains(f.msg, "dial") {
		t.Fatalf("network msg = %q", f.msg)
	}
	f := classify(&ackError{err: client.ErrNetwork})
	if !strings.Contains(f.msg, "retrieved") {
		t.Fatalf("ack msg = %q", f.msg)
	}
	if !errors.Is(&ackError{err: client.ErrNetwork}, client.ErrNetwork) {
		t.Fatal("ackError does not unwrap")
	}
	if !errors.Is(ioErr("x", client.ErrNetwork), client.ErrNetwork) {
		t.Fatal("ioError does not unwrap")
	}
	if rl := classify(&client.RateLimitError{RetryAfter: 3 * time.Second}); rl.retryAfter != 3*time.Second {
		t.Fatalf("retryAfter = %v", rl.retryAfter)
	}
}
