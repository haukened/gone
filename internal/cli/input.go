package cli

import (
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/haukened/gone/v3/internal/envelope"
)

// maxInputBytes caps the message plus attachments before encryption.
const maxInputBytes = 64 << 20

// messagePrompt is shown on a terminal before reading the message.
const messagePrompt = "Type the message, then press Ctrl-D on a new line (Ctrl-Z then Enter on Windows).\n"

// maxLinkInput caps how much of stdin is read for a "-" link argument.
const maxLinkInput = 4 << 10

// linkPrompt is shown on a terminal before reading a "-" link argument.
const linkPrompt = "Paste the link, then press Ctrl-D on a new line (Ctrl-Z then Enter on Windows).\n"

// readPayload reads the attachments and then the message, from messageFile
// when it is set and from stdin otherwise. The caller must clear the
// returned data.
//
// Parameters:
//   - paths: attachment paths.
//   - messageFile: --message-file value, or "" to read stdin.
//
// Returns the payload or a usage/I/O error.
func (a *app) readPayload(paths []string, messageFile string) (envelope.Payload, error) {
	var p envelope.Payload
	if len(paths) > envelope.MaxFiles {
		return p, usagef("at most %d files can be attached", envelope.MaxFiles)
	}
	budget := int64(maxInputBytes)
	for _, path := range paths {
		f, err := readAttachment(path, budget)
		if err != nil {
			clearPayload(p)
			return envelope.Payload{}, err
		}
		budget -= int64(len(f.Data))
		p.Files = append(p.Files, f)
	}
	msg, err := a.readMessage(budget, messageFile)
	if err != nil {
		clearPayload(p)
		return envelope.Payload{}, err
	}
	p.Message = msg
	if len(msg) == 0 && len(p.Files) == 0 {
		return p, usagef("nothing to send: the message is empty and no files are attached")
	}
	return p, nil
}

// readMessage reads the message up to budget bytes: from messageFile when
// it is set, without touching stdin, otherwise from stdin, prompting on a
// terminal.
//
// Parameters:
//   - budget: remaining input allowance.
//   - messageFile: --message-file value, or "" to read stdin.
//
// Returns the message or a usage/I/O error.
func (a *app) readMessage(budget int64, messageFile string) ([]byte, error) {
	if messageFile != "" {
		return readRegularFile(messageFile, budget, "message file")
	}
	if a.env.StdinTTY {
		_, _ = io.WriteString(a.env.Stderr, messagePrompt)
	}
	msg, err := io.ReadAll(io.LimitReader(a.env.Stdin, budget+1))
	if err != nil {
		clear(msg)
		return nil, ioErr("read message", err)
	}
	if int64(len(msg)) > budget {
		clear(msg)
		return nil, tooLargeInput()
	}
	return msg, nil
}

// readAttachment reads one regular file within budget.
//
// Parameters:
//   - path: file path.
//   - budget: remaining input allowance.
//
// Returns the attachment or a usage/I/O error.
func readAttachment(path string, budget int64) (envelope.File, error) {
	data, err := readRegularFile(path, budget, "attachment")
	if err != nil {
		return envelope.File{}, err
	}
	name := strings.ToValidUTF8(filepath.Base(path), "_")
	return envelope.File{Name: name, Type: mime.TypeByExtension(filepath.Ext(path)), Data: data}, nil
}

// readRegularFile reads one regular file within budget. Errors name the
// path but never include its contents. The caller must clear the result.
//
// Parameters:
//   - path: file path.
//   - budget: remaining input allowance.
//   - what: file role for error messages, such as "attachment".
//
// Returns the contents or a usage/I/O error.
func readRegularFile(path string, budget int64, what string) ([]byte, error) {
	fh, err := os.Open(path) // #nosec G304 -- path chosen by the user
	if err != nil {
		return nil, ioErr("open "+what, err)
	}
	defer func() { _ = fh.Close() }()
	info, err := fh.Stat()
	if err != nil {
		return nil, ioErr("open "+what, err)
	}
	if !info.Mode().IsRegular() {
		return nil, usagef("%s is not a regular file", path)
	}
	if info.Size() > budget {
		return nil, tooLargeInput()
	}
	data, err := io.ReadAll(io.LimitReader(fh, budget+1))
	if err != nil {
		clear(data)
		return nil, ioErr("read "+what, err)
	}
	if int64(len(data)) > budget {
		clear(data)
		return nil, tooLargeInput()
	}
	return data, nil
}

// readLinkArg resolves a link argument: "-" reads the link from stdin
// (prompting on a terminal) so it stays out of the process list and shell
// history; anything else is returned unchanged. The input is never echoed
// in errors.
//
// Parameters:
//   - raw: positional link argument.
//
// Returns the link with surrounding whitespace removed, or a usage/I/O
// error.
func (a *app) readLinkArg(raw string) (string, error) {
	if raw != "-" {
		return raw, nil
	}
	if a.env.StdinTTY {
		_, _ = io.WriteString(a.env.Stderr, linkPrompt)
	}
	b, err := io.ReadAll(io.LimitReader(a.env.Stdin, maxLinkInput+1))
	if err != nil {
		return "", ioErr("read link", err)
	}
	if len(b) > maxLinkInput {
		return "", usagef("the link on standard input is too long")
	}
	return strings.TrimSpace(string(b)), nil
}

// tooLargeInput reports that the inputs exceed maxInputBytes.
//
// Returns a usage error.
func tooLargeInput() error {
	return usagef("the message and files together exceed %d MiB", maxInputBytes>>20)
}

// clearPayload zeroes the message and attachment bytes.
//
// Parameters:
//   - p: payload to clear.
func clearPayload(p envelope.Payload) {
	clear(p.Message)
	for _, f := range p.Files {
		clear(f.Data)
	}
}
