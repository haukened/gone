package cli

import (
	"io"
	"os"
	"runtime"
	"unicode/utf8"

	"github.com/haukened/gone/v3/internal/domain"
)

// promptPassword reads one passphrase from the terminal without echo.
//
// Parameters:
//   - label: prompt written to stderr.
//
// Returns the passphrase or a usage/I/O error.
func (a *app) promptPassword(label string) ([]byte, error) {
	p, err := a.readPassword(label)
	if err != nil {
		return nil, err
	}
	if err := checkPassphrase(p); err != nil {
		clear(p)
		return nil, err
	}
	return p, nil
}

// readPassword reads one line from the terminal without echo and without
// validating it.
//
// Parameters:
//   - label: prompt written to stderr.
//
// Returns the raw input or a usage/I/O error.
func (a *app) readPassword(label string) ([]byte, error) {
	if !a.env.StdinTTY {
		return nil, usagef("a passphrase prompt needs an interactive terminal; use --passphrase-file%s",
			gitBashHint(runtime.GOOS, a.env.Getenv("MSYSTEM")))
	}
	_, _ = io.WriteString(a.env.Stderr, label)
	p, err := a.env.ReadPassword()
	_, _ = io.WriteString(a.env.Stderr, "\n")
	if err != nil {
		return nil, ioErr("read passphrase", err)
	}
	return p, nil
}

// readPassphraseFile reads a passphrase file as text (see decodeText, so a
// file saved as UTF-16 or with a byte-order mark still works), stripping
// exactly one trailing "\n" or "\r\n".
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
	// Room for the longest passphrase in UTF-16, a BOM and "\r\n".
	p, err := io.ReadAll(io.LimitReader(f, 2*domain.V2MaxPassphraseBytes+8))
	if err != nil {
		clear(p)
		return nil, ioErr("read passphrase file", err)
	}
	p, _, err = decodeText(p)
	if err != nil {
		return nil, err
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

// gitBashHint explains, in Git Bash and other MSYS terminals on Windows,
// why gone sees no terminal: they connect programs through pipes, not a
// Windows console.
//
// Parameters:
//   - goos: the operating system, as in runtime.GOOS.
//   - msystem: the MSYSTEM environment variable, which MSYS sets.
//
// Returns the hint, starting with a space, or "".
func gitBashHint(goos, msystem string) string {
	if goos != "windows" || msystem == "" {
		return ""
	}
	return " (in Git Bash, run it as \"winpty gone ...\", or use Windows Terminal)"
}
