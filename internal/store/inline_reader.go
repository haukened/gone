package store

import "io"

// inlineReader provides a zero-allocation Read over a byte slice.
type inlineReader struct {
	b []byte
}

// newInlineReader wraps b in an inlineReader.
//
// Parameters:
//   - b: byte slice to read.
//
// Returns a reader over b.
func newInlineReader(b []byte) *inlineReader { return &inlineReader{b: b} }

// Read copies bytes into p and advances the reader.
//
// Parameters:
//   - p: destination buffer.
//
// Returns the number of bytes copied and io.EOF when exhausted.
func (r *inlineReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
