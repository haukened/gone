package cli

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSaveName checks Windows-safe renaming of attachment names.
//
// Parameters:
//   - t: the test.
func TestSaveName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"notes.txt", "notes.txt"},
		{`a<b>c:d"e|f?g*h`, "a_b_c_d_e_f_g_h"},
		{"trailing. . ", "trailing"},
		{"...", fallbackSaveName},
		{"", fallbackSaveName},
		{"CON", "_CON"},
		{"con.txt", "_con.txt"},
		{"nul .tar.gz", "_nul .tar.gz"},
		{"CONIN$", "_CONIN$"},
		{"COM1", "_COM1"},
		{"lpt9.log", "_lpt9.log"},
		{"COM0", "COM0"},
		{"COM10", "COM10"},
		{"COM¹", "_COM¹"},
		{"lpt³.txt", "_lpt³.txt"},
		{"COMX", "COMX"},
		{"FOO1", "FOO1"},
		{"CO", "CO"},
		{"console", "console"},
		{".zshenv", "_zshenv"},
		{".gitconfig", "_gitconfig"},
		{"..bashrc", "_.bashrc"},
		{"-rf", "_rf"},
		{"--help.txt", "_-help.txt"},
		{"-", "_"},
	}
	for _, tt := range tests {
		if got := saveName(tt.in); got != tt.want {
			t.Errorf("saveName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// FuzzSaveName checks that saveName never returns an empty name, a name
// with Windows-invalid characters, one starting with a dot or dash, or one
// ending in a dot or space.
//
// Parameters:
//   - f: the fuzzer.
func FuzzSaveName(f *testing.F) {
	for _, s := range []string{"a.txt", "CON", "x. ", "<>", "COM¹.x", ".profile", "-x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			t.Skip()
		}
		if got := saveName(in); !safeSaveName(got) {
			t.Fatalf("saveName(%q) = %q", in, got)
		}
	})
}

// safeSaveName reports whether name satisfies every saveName invariant.
//
// Parameters:
//   - name: a saveName result.
//
// Returns:
//   - true if name is non-empty, free of Windows-invalid characters, does
//     not start with a dot or dash, does not end in a dot or space, and is
//     not a reserved device name.
func safeSaveName(name string) bool {
	if name == "" || strings.ContainsAny(name, windowsInvalid) {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") {
		return false
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	return !isReservedStem(name)
}
