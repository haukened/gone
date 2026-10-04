package cli

import (
	"crypto/subtle"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/passgen"
)

// minPassphraseRunes matches the web UI's minimum (web/js/passgen.js
// MIN_CHARS) for passphrases chosen by the sender.
const minPassphraseRunes = 8

// passOpts holds the mutually exclusive passphrase source flags.
type passOpts struct {
	prompt   bool
	file     string
	generate bool
}

// register adds the passphrase flags to fs.
//
// Parameters:
//   - fs: flag set to extend.
//   - send: whether to add the send-only flags.
func (p *passOpts) register(fs flagAdder, send bool) {
	fs.StringVar(&p.file, "passphrase-file", "", "")
	if send {
		fs.BoolVar(&p.prompt, "passphrase-prompt", false, "")
		fs.BoolVar(&p.generate, "passphrase-generate", false, "")
	}
}

// flagAdder is the subset of *flag.FlagSet used to register flags.
type flagAdder interface {
	StringVar(p *string, name, value, usage string)
	BoolVar(p *bool, name string, value bool, usage string)
}

// sendPassphrase resolves the sender's passphrase. Nil means protocol v1.
// The caller must clear the result.
//
// Parameters:
//   - o: passphrase flags.
//
// Returns the passphrase, whether it was generated, or a usage/I/O error.
func (a *app) sendPassphrase(o passOpts) ([]byte, bool, error) {
	switch {
	case countTrue(o.prompt, o.file != "", o.generate) > 1:
		return nil, false, usagef("choose only one of --passphrase-prompt, --passphrase-file, --passphrase-generate")
	case o.generate:
		p, err := passgen.Generate()
		if err != nil {
			return nil, false, fmt.Errorf("generate passphrase: %w", err)
		}
		return []byte(p), true, nil
	case o.file != "":
		p, err := senderPassphraseFile(o.file)
		return p, false, err
	case o.prompt:
		p, err := a.promptNewPassphrase()
		return p, false, err
	}
	return nil, false, nil
}

// countTrue counts the true values in flags.
//
// Parameters:
//   - flags: booleans to count.
//
// Returns the number of true values.
func countTrue(flags ...bool) int {
	n := 0
	for _, set := range flags {
		if set {
			n++
		}
	}
	return n
}

// senderPassphraseFile reads a sender passphrase file and enforces the
// minimum length. The caller must clear the result.
//
// Parameters:
//   - path: file path.
//
// Returns the passphrase or a usage/I/O error.
func senderPassphraseFile(path string) ([]byte, error) {
	p, err := readPassphraseFile(path)
	if err == nil {
		err = checkMinLength(p)
	}
	if err != nil {
		clear(p)
		return nil, err
	}
	return p, nil
}

// promptNewPassphrase asks for a passphrase twice on the terminal.
//
// Returns the passphrase or a usage/I/O error.
func (a *app) promptNewPassphrase() ([]byte, error) {
	p, err := a.promptPassword("Passphrase: ")
	if err != nil {
		return nil, err
	}
	if err := checkMinLength(p); err != nil {
		clear(p)
		return nil, err
	}
	confirm, err := a.promptPassword("Confirm passphrase: ")
	defer clear(confirm)
	if err != nil {
		clear(p)
		return nil, err
	}
	if subtle.ConstantTimeCompare(p, confirm) != 1 {
		clear(p)
		return nil, usagef("passphrases do not match")
	}
	return p, nil
}

// promptPassword reads one passphrase from the terminal without echo.
//
// Parameters:
//   - label: prompt written to stderr.
//
// Returns the passphrase or a usage/I/O error.
func (a *app) promptPassword(label string) ([]byte, error) {
	if !a.env.StdinTTY {
		return nil, usagef("a passphrase prompt needs an interactive terminal; use --passphrase-file")
	}
	_, _ = io.WriteString(a.env.Stderr, label)
	p, err := a.env.ReadPassword()
	_, _ = io.WriteString(a.env.Stderr, "\n")
	if err != nil {
		return nil, ioErr("read passphrase", err)
	}
	if err := checkPassphrase(p); err != nil {
		clear(p)
		return nil, err
	}
	return p, nil
}

// readPassphraseFile reads a passphrase file, stripping exactly one
// trailing "\n" or "\r\n".
//
// Parameters:
//   - path: file path.
//
// Returns the passphrase or a usage/I/O error.
func readPassphraseFile(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- path chosen by the user
	if err != nil {
		return nil, ioErr("read passphrase file", err)
	}
	defer func() { _ = f.Close() }()
	p, err := io.ReadAll(io.LimitReader(f, domain.V2MaxPassphraseBytes+3))
	if err != nil {
		clear(p)
		return nil, ioErr("read passphrase file", err)
	}
	trimmed := trimNewline(p)
	if err := checkPassphrase(trimmed); err != nil {
		clear(p)
		return nil, err
	}
	return trimmed, nil
}

// trimNewline removes one trailing "\n" or "\r\n".
//
// Parameters:
//   - p: bytes to trim.
//
// Returns a subslice of p.
func trimNewline(p []byte) []byte {
	n := len(p)
	if n > 0 && p[n-1] == '\n' {
		n--
		if n > 0 && p[n-1] == '\r' {
			n--
		}
	}
	return p[:n]
}

// checkPassphrase validates a passphrase's encoding and size.
//
// Parameters:
//   - p: passphrase.
//
// Returns a usage error when invalid.
func checkPassphrase(p []byte) error {
	if len(p) == 0 || len(p) > domain.V2MaxPassphraseBytes || !utf8.Valid(p) {
		return usagef("the passphrase must be 1 to %d bytes of valid UTF-8", domain.V2MaxPassphraseBytes)
	}
	return nil
}

// checkMinLength enforces the sender minimum length.
//
// Parameters:
//   - p: passphrase.
//
// Returns a usage error when too short.
func checkMinLength(p []byte) error {
	if utf8.RuneCount(p) < minPassphraseRunes {
		return usagef("the passphrase must be at least %d characters", minPassphraseRunes)
	}
	return nil
}
