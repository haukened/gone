package cli

import (
	"crypto/subtle"
	"fmt"

	"github.com/haukened/gone/v3/internal/passgen"
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

// set reports whether any passphrase source flag was given.
//
// Returns true when the sender chose a passphrase.
func (p passOpts) set() bool {
	return countTrue(p.prompt, p.file != "", p.generate) > 0
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
