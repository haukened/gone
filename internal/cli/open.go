package cli

import (
	"errors"
	"io"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// maxPassphraseAttempts bounds interactive passphrase retries.
const maxPassphraseAttempts = 3

// passphraseLabel prompts for a recipient passphrase.
const passphraseLabel = "Passphrase: "

// decrypt opens a claimed secret. For v2 it uses pass when non-nil (one
// attempt), else prompts up to maxPassphraseAttempts times. The caller must
// clear the result.
//
// Parameters:
//   - version: link protocol version.
//   - key: link key.
//   - pass: passphrase from a file, or nil to prompt.
//   - c: claimed ciphertext.
//
// Returns the plaintext, errWrongPassphrase, or an integrity/usage error.
func (a *app) decrypt(version uint8, key, pass []byte, c client.Claimed) ([]byte, error) {
	if version == domain.ProtocolV1 {
		return envelope.Open(key, c.Nonce, c.Body)
	}
	attempts := maxPassphraseAttempts
	if pass != nil {
		attempts = 1
	}
	for i := range attempts {
		pt, err := a.tryPassphrase(key, pass, c)
		retry := "Wrong passphrase, try again.\n"
		var bad *invalidEntryError
		switch {
		case errors.As(err, &bad):
			retry = "Invalid passphrase: " + bad.msg + ", try again.\n"
		case !errors.Is(err, envelope.ErrDecrypt):
			return pt, err
		}
		if i < attempts-1 {
			_, _ = io.WriteString(a.env.Stderr, retry)
		}
	}
	return nil, errWrongPassphrase
}

// invalidEntryError reports a prompted passphrase that could never be right
// (empty, too long, or not UTF-8). The secret is already claimed when the
// prompt runs, so this counts as a failed attempt instead of ending the
// command, which would abandon the claim and lose the secret.
type invalidEntryError struct{ msg string }

// Error returns the reason the entry was rejected.
//
// Returns the message.
func (e *invalidEntryError) Error() string { return e.msg }

// tryPassphrase makes one v2 decryption attempt, prompting when pass is nil.
//
// Parameters:
//   - key: link key.
//   - pass: passphrase, or nil to prompt.
//   - c: claimed ciphertext.
//
// Returns the plaintext or an error (envelope.ErrDecrypt on a wrong passphrase,
// *invalidEntryError on an entry that fails checkPassphrase).
func (a *app) tryPassphrase(key, pass []byte, c client.Claimed) ([]byte, error) {
	p := pass
	if p == nil {
		var err error
		if p, err = a.readPassword(passphraseLabel); err != nil {
			return nil, err
		}
		defer clear(p)
		if err := checkPassphrase(p); err != nil {
			return nil, &invalidEntryError{msg: err.Error()}
		}
	}
	return envelope.OpenV2(key, string(p), c.Nonce, c.Body)
}

// recipientPassphrase gathers what a v2 link needs before the claim: the
// file passphrase, or confirmation that a prompt is possible.
//
// Parameters:
//   - version: link protocol version.
//   - file: --passphrase-file value.
//
// Returns the file passphrase (nil means prompt or v1) or a usage/I/O error.
func (a *app) recipientPassphrase(version uint8, file string) ([]byte, error) {
	switch {
	case version == domain.ProtocolV1 && file != "":
		return nil, usagef("this link has no passphrase; remove --passphrase-file")
	case version == domain.ProtocolV1:
		return nil, nil
	case file != "":
		return readPassphraseFile(file)
	case !a.env.StdinTTY:
		return nil, usagef("this link needs a passphrase; use --passphrase-file or run in a terminal")
	}
	return nil, nil
}
