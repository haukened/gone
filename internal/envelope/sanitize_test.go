package envelope

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFileName(t *testing.T) {
	long := strings.Repeat("é", 300)
	cases := []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{"", "file"},
		{".", "file"},
		{"..", "file"},
		{"../../etc/passwd", "passwd"},
		{`C:\Users\a\evil.exe`, "evil.exe"},
		{"dir/", "dir"},
		{"/", "file"},
		{"a/..", "file"},
		{"  name.txt \t", "name.txt"},
		{"\u00a0\u3000x\ufeff", "x"},
		{"a\x00b\x1fc\x7fd\u0085e", "abcde"},
		{"invoice\u202etxt.exe", "invoicetxt.exe"},
		{"a\u200bb\u200fc\u2066d\u206fe\u061cf\u180eg", "abcdefg"},
		{"\u200b..\u200b", "file"},
		{"\xff\xfe", "\ufffd\ufffd"},
		{long, strings.Repeat("é", 255)},
		{strings.Repeat("\u200b", 300) + "ok", "ok"},
		{strings.Repeat("a", 254) + " b", strings.Repeat("a", 254)},
		{"日本語.txt", "日本語.txt"},
		{"emoji😀.png", "emoji😀.png"},
	}
	for _, c := range cases {
		got := SanitizeFileName(c.in)
		if got != c.want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", c.in, got, c.want)
		}
		if SanitizeFileName(got) != got {
			t.Errorf("not idempotent for %q", c.in)
		}
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) > maxNameRunes {
			t.Errorf("invalid output for %q", c.in)
		}
	}
}
