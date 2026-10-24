package cli

import (
	"bytes"
	"testing"
)

// TestDecodeText checks byte-order mark handling: UTF-8 BOMs are dropped,
// UTF-16 in either byte order becomes UTF-8 (surrogate pairs included),
// UTF-16 with an odd length is refused, and anything else is unchanged.
//
// Parameters:
//   - t: the test handle.
func TestDecodeText(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
		did  bool
	}{
		{"plain", []byte("héllo\n"), "héllo\n", false},
		{"empty", nil, "", false},
		{"utf-8 bom", []byte("\xEF\xBB\xBFpass"), "pass", true},
		{"utf-16le", []byte{0xFF, 0xFE, 'p', 0, 0xE9, 0, '\n', 0}, "pé\n", true},
		{"utf-16be", []byte{0xFE, 0xFF, 0, 'p', 0, 0xE9}, "pé", true},
		{"surrogate pair", []byte{0xFF, 0xFE, 0x3D, 0xD8, 0x11, 0xDD}, "\U0001F511", true},
		{"bom only", []byte{0xFF, 0xFE}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bytes.Clone(tt.in)
			got, did, err := decodeText(in)
			if err != nil || string(got) != tt.want || (did != "") != tt.did {
				t.Fatalf("decodeText = %q, %q, %v; want %q", got, did, err, tt.want)
			}
		})
	}
	odd := []byte{0xFF, 0xFE, 'p', 0, 'x'}
	if _, _, err := decodeText(odd); classify(err).code != exitUsage {
		t.Fatalf("odd length: %v", err)
	}
	if !bytes.Equal(odd, make([]byte, len(odd))) {
		t.Fatalf("odd-length input not cleared: %v", odd)
	}
	in := []byte{0xFF, 0xFE, 's', 0}
	if _, _, err := decodeText(in); err != nil || !bytes.Equal(in, make([]byte, len(in))) {
		t.Fatalf("converted input not cleared: %v, %v", in, err)
	}
}
