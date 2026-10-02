package envelope

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"
)

const (
	// MaxFiles is the maximum number of attachments in one envelope.
	MaxFiles = 10
	// MaxHeaderBytes is the maximum GONE2 header length.
	MaxHeaderBytes = 16 * 1024
	// prefixLen is the magic plus the 4-byte header length.
	prefixLen = 12
	// maxSafeInt is 2^53-1, the largest size both Go and JS represent exactly.
	maxSafeInt = 1<<53 - 1
)

// Magic is the 8-byte prefix that marks a GONE2 envelope.
var Magic = [8]byte{'G', 'O', 'N', 'E', '2', 0, 0, 0}

// File is one attachment.
type File struct {
	// Name is the file name; sanitized on Pack and Unpack.
	Name string
	// Type is the MIME type; mapped onto the allowlist on Pack and Unpack.
	Type string
	// Data is the file content.
	Data []byte
}

// Payload is a decrypted secret: a message and optional attachments.
type Payload struct {
	// Message is the UTF-8 message bytes.
	Message []byte
	// Files are the attachments, in envelope order.
	Files []File
}

// fileHeader is one sanitized "files" entry of a GONE2 header.
type fileHeader struct {
	name string
	typ  string
	size uint64
}

// needsEnvelope reports whether a payload must be encoded as GONE2: it has
// attachments, or its message would otherwise be mistaken for one.
//
// Parameters:
//   - msg: message bytes.
//   - nFiles: number of attachments.
//
// Returns true when Pack must emit GONE2.
func needsEnvelope(msg []byte, nFiles int) bool {
	return nFiles > 0 || bytes.HasPrefix(msg, Magic[:])
}

// Pack encodes p as plaintext (docs/protocol.md §5.4): raw message bytes
// when possible, otherwise a canonical GONE2 envelope. Names and types are
// sanitized.
//
// Parameters:
//   - p: payload to encode.
//
// Returns the plaintext, or ErrTooManyFiles, ErrInvalidMetadata, or
// ErrHeaderTooLarge.
func Pack(p Payload) ([]byte, error) {
	if len(p.Files) > MaxFiles {
		return nil, ErrTooManyFiles
	}
	if !needsEnvelope(p.Message, len(p.Files)) {
		return bytes.Clone(p.Message), nil
	}
	hdrs, err := packFileHeaders(p.Files)
	if err != nil {
		return nil, err
	}
	header, err := buildHeader(len(p.Message), hdrs)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, envelopeLen(len(header), p))
	out = append(out, Magic[:]...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(header))) // #nosec G115 -- len <= MaxHeaderBytes
	out = append(out, header...)
	out = append(out, p.Message...)
	for _, f := range p.Files {
		out = append(out, f.Data...)
	}
	return out, nil
}

// packFileHeaders validates and sanitizes attachment metadata.
//
// Parameters:
//   - files: attachments.
//
// Returns sanitized headers or ErrInvalidMetadata.
func packFileHeaders(files []File) ([]fileHeader, error) {
	hdrs := make([]fileHeader, len(files))
	for i, f := range files {
		if !utf8.ValidString(f.Name) || !utf8.ValidString(f.Type) {
			return nil, ErrInvalidMetadata
		}
		hdrs[i] = fileHeader{name: SanitizeFileName(f.Name), typ: SafeType(f.Type), size: uint64(len(f.Data))}
	}
	return hdrs, nil
}

// buildHeader serializes the canonical header and enforces MaxHeaderBytes.
// Sanitization currently bounds headers well below the cap; the check
// remains because docs/protocol.md §5.4 requires it.
//
// Parameters:
//   - msgLen: message length.
//   - hdrs: sanitized file headers.
//
// Returns the header bytes or ErrHeaderTooLarge.
func buildHeader(msgLen int, hdrs []fileHeader) ([]byte, error) {
	header := canonicalHeader(msgLen, hdrs)
	if len(header) > MaxHeaderBytes {
		return nil, ErrHeaderTooLarge
	}
	return header, nil
}

// envelopeLen returns the total GONE2 plaintext length.
//
// Parameters:
//   - headerLen: canonical header length.
//   - p: payload.
//
// Returns the byte length.
func envelopeLen(headerLen int, p Payload) int {
	n := prefixLen + headerLen + len(p.Message)
	for _, f := range p.Files {
		n += len(f.Data)
	}
	return n
}
