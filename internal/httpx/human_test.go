package httpx

import "testing"

func TestHumanBytes(t *testing.T) {
	tests := []struct{ in int64; expect string }{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1024 + 512, "1.5 KB"},
		{1024*1024 - 1, "1024.0 KB"}, // just under 1 MiB (shows KB rollover logic)
		{1024 * 1024, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
	}
	for _, tc := range tests {
		if got := humanBytes(tc.in); got != tc.expect {
				 t.Fatalf("humanBytes(%d) expected %q got %q", tc.in, tc.expect, got)
		}
	}
}

func TestHumanTTL(t *testing.T) {
	tests := []struct{ in int; expect string }{
		{0, "0s"},
		{-5, "0s"},
		{59, "59s"},
		{60, "1m"},
		{61, "61s"},
		{180, "3m"},
		{3600, "1h"},
		{7200, "2h"},
	}
	for _, tc := range tests {
		if got := humanTTL(tc.in); got != tc.expect {
			 t.Fatalf("humanTTL(%d) expected %q got %q", tc.in, tc.expect, got)
		}
	}
}
