package cli

import (
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/haukened/gone/internal/envelope"
)

// maxInputBytes caps the message plus attachments before encryption.
const maxInputBytes = 64 << 20

// messagePrompt is shown on a terminal before reading the message.
const messagePrompt = "Type the message, then press Ctrl-D on a new line (Ctrl-Z then Enter on Windows).\n"

// readPayload reads the attachments and then the message from stdin. The
// caller must clear the returned data.
//
// Parameters:
//   - paths: attachment paths.
//
// Returns the payload or a usage/I/O error.
func (a *app) readPayload(paths []string) (envelope.Payload, error) {
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
	msg, err := a.readMessage(budget)
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

// readMessage reads stdin up to budget bytes, prompting on a terminal.
//
// Parameters:
//   - budget: remaining input allowance.
//
// Returns the message or a usage/I/O error.
func (a *app) readMessage(budget int64) ([]byte, error) {
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
	fh, err := os.Open(path) // #nosec G304 -- path chosen by the user
	if err != nil {
		return envelope.File{}, ioErr("open attachment", err)
	}
	defer func() { _ = fh.Close() }()
	info, err := fh.Stat()
	if err != nil {
		return envelope.File{}, ioErr("open attachment", err)
	}
	if !info.Mode().IsRegular() {
		return envelope.File{}, usagef("%s is not a regular file", path)
	}
	if info.Size() > budget {
		return envelope.File{}, tooLargeInput()
	}
	data, err := io.ReadAll(io.LimitReader(fh, budget+1))
	if err != nil {
		clear(data)
		return envelope.File{}, ioErr("read attachment", err)
	}
	if int64(len(data)) > budget {
		clear(data)
		return envelope.File{}, tooLargeInput()
	}
	name := strings.ToValidUTF8(filepath.Base(path), "_")
	return envelope.File{Name: name, Type: mime.TypeByExtension(filepath.Ext(path)), Data: data}, nil
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
