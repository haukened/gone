package envelope

import (
	"testing"
)

func TestSafeType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"application/pdf", "application/pdf"},
		{"Image/PNG", "image/png"},
		{" text/plain ; charset=utf-8", "text/plain"},
		{"text/plain;", "text/plain"},
		{"text/html", DefaultType},
		{"image/svg+xml", DefaultType},
		{"", DefaultType},
		{"application/x-7z-compressed", "application/x-7z-compressed"},
		{"text/plaın", DefaultType},
		{"TEXT/PLAİN", DefaultType},
		{"\u00a0text/csv\u3000", "text/csv"},
		{"text/csv\x00", DefaultType},
	}
	for _, c := range cases {
		if got := SafeType(c.in); got != c.want {
			t.Errorf("SafeType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for typ := range safeTypes {
		if SafeType(typ) != typ {
			t.Errorf("allowlisted %q not preserved", typ)
		}
	}
}
