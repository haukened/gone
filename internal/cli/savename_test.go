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
		{"con.TXT", "_con.TXT"},
		{"Lpt1.log", "_Lpt1.log"},
		{"cOm9.Tar.Gz", "_cOm9.Tar.Gz"},
		{"NuL.", "_NuL"},
		{"a?.txt", "a_.txt"},
		{"a*.txt", "a_.txt"},
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
	for _, s := range []string{strings.Repeat("é", 255), strings.Repeat("a", 300) + ".txt", "a.txt", "CON", "x. ", "<>", "COM¹.x", ".profile", "-x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			t.Skip()
		}
		if got := saveName(in); !safeSaveName(got) || len(got) > maxSaveNameBytes+1 || !utf8.ValidString(got) {
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

// TestSaveNameShortens checks long names are cut to a valid UTF-8 prefix
// that keeps a short extension, so name and collision suffix fit in the 255
// bytes file systems allow.
func TestSaveNameShortens(t *testing.T) {
	tests := []struct {
		name, in string
		wantExt  string
	}{
		{"255 two-byte runes", strings.Repeat("é", 255), ""},
		{"255 four-byte runes", strings.Repeat("\U0001F600", 255), ""},
		{"long ASCII with extension", strings.Repeat("a", 400) + ".txt", ".txt"},
		{"long multibyte stem with extension", strings.Repeat("日", 200) + ".tar.gz", ".gz"},
		{"extension too long to keep", "x" + strings.Repeat(".y", 20) + "." + strings.Repeat("z", 250), ""},
		{"stem ends in dot after cut", strings.Repeat("a", 199) + "..txt", ".txt"},
	}
	for _, tt := range tests {
		got := saveName(tt.in)
		if len(got) > maxSaveNameBytes+1 || len(got)+len(" (999)") > 255 {
			t.Errorf("%s: %d bytes", tt.name, len(got))
		}
		if !utf8.ValidString(got) || !safeSaveName(got) {
			t.Errorf("%s: unsafe result %q", tt.name, got)
		}
		if !strings.HasSuffix(got, tt.wantExt) {
			t.Errorf("%s: %q lost extension %q", tt.name, got, tt.wantExt)
		}
		if len(tt.in) > maxSaveNameBytes && len(got) < maxSaveNameBytes-utf8.UTFMax-maxExtBytes && tt.wantExt != "" {
			t.Errorf("%s: cut too much: %d bytes", tt.name, len(got))
		}
	}
	if got := saveName(strings.Repeat("a", 200) + ".b"); got != strings.Repeat("a", 198)+".b" {
		t.Errorf("exact fit: got %d bytes", len(got))
	}
	if got := saveName(strings.Repeat("a", 200)); got != strings.Repeat("a", 200) {
		t.Errorf("name at the limit changed")
	}
}
