package ratelimit

import (
	"net/netip"
	"strings"
	"testing"
)

func TestClientKey(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("fd00::/8"),
	}
	tests := []struct {
		name          string
		remote        string
		xff           []string
		trusted       []netip.Prefix
		want          string
		wantUntrusted bool
	}{
		{name: "ipv4 peer", remote: "192.0.2.7:5555", want: "192.0.2.7/32"},
		{name: "ipv6 peer masked to /64", remote: "[2001:db8:1:2:3:4:5:6]:443", want: "2001:db8:1:2::/64"},
		{name: "ipv6 zone stripped", remote: "[fe80::1%eth0]:443", want: "fe80::/64"},
		{name: "ipv4-mapped unmapped", remote: "[::ffff:192.0.2.9]:80", want: "192.0.2.9/32"},
		{name: "bare ip peer", remote: "192.0.2.8", want: "192.0.2.8/32"},
		{name: "unparsable peer", remote: "not-an-ip", want: "::/128"},
		{name: "empty peer", remote: "", want: "::/128"},
		{name: "unparsable peer flags xff", remote: "", xff: []string{"192.0.2.1"}, want: "::/128", wantUntrusted: true},
		{name: "untrusted peer ignores xff", remote: "192.0.2.7:1", xff: []string{"198.51.100.1"}, trusted: trusted, want: "192.0.2.7/32", wantUntrusted: true},
		{name: "no trusted list ignores xff", remote: "10.0.0.1:1", xff: []string{"198.51.100.1"}, want: "10.0.0.1/32", wantUntrusted: true},
		{name: "trusted peer uses xff", remote: "10.0.0.1:1", xff: []string{"198.51.100.1"}, trusted: trusted, want: "198.51.100.1/32"},
		{name: "rightmost untrusted wins", remote: "10.0.0.1:1", xff: []string{"203.0.113.5, 198.51.100.1"}, trusted: trusted, want: "198.51.100.1/32"},
		{name: "trusted hops skipped", remote: "10.0.0.1:1", xff: []string{"198.51.100.1, 10.2.2.2, 10.3.3.3"}, trusted: trusted, want: "198.51.100.1/32"},
		{name: "multiple header values", remote: "10.0.0.1:1", xff: []string{"198.51.100.1", "10.2.2.2"}, trusted: trusted, want: "198.51.100.1/32"},
		{name: "spoofed leftmost ignored", remote: "10.0.0.1:1", xff: []string{"1.2.3.4,198.51.100.1"}, trusted: trusted, want: "198.51.100.1/32"},
		{name: "hop with port", remote: "10.0.0.1:1", xff: []string{"198.51.100.1:9000"}, trusted: trusted, want: "198.51.100.1/32"},
		{name: "ipv6 hop", remote: "[fd00::1]:1", xff: []string{"2001:db8:aa:bb::1"}, trusted: trusted, want: "2001:db8:aa:bb::/64"},
		{name: "invalid hop uses last trusted", remote: "10.0.0.1:1", xff: []string{"198.51.100.1, garbage, 10.2.2.2"}, trusted: trusted, want: "10.2.2.2/32"},
		{name: "invalid nearest hop uses peer", remote: "10.0.0.1:1", xff: []string{"garbage"}, trusted: trusted, want: "10.0.0.1/32"},
		{name: "empty entry uses peer", remote: "10.0.0.1:1", xff: []string{""}, trusted: trusted, want: "10.0.0.1/32"},
		{name: "all trusted uses farthest", remote: "10.0.0.1:1", xff: []string{"10.9.9.9, 10.2.2.2"}, trusted: trusted, want: "10.9.9.9/32"},
		{name: "trusted peer no xff", remote: "10.0.0.1:1", trusted: trusted, want: "10.0.0.1/32"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, untrusted := ClientKey(tc.remote, tc.xff, tc.trusted)
			if got.String() != tc.want || untrusted != tc.wantUntrusted {
				t.Fatalf("ClientKey() = (%s, %v), want (%s, %v)", got, untrusted, tc.want, tc.wantUntrusted)
			}
		})
	}
}

func TestClientKeyHopLimit(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	hops := make([]string, 0, maxForwardedHops+1)
	hops = append(hops, "198.51.100.1")
	for i := 0; i < maxForwardedHops; i++ {
		hops = append(hops, "10.1.1.1")
	}
	xff := strings.Join(hops, ",")
	got, _ := ClientKey("10.0.0.1:1", []string{xff}, trusted)
	if got.String() != "10.1.1.1/32" {
		t.Fatalf("ClientKey() = %s, want walk to stop at %d hops", got, maxForwardedHops)
	}
}

func TestForwardedFromRight(t *testing.T) {
	tests := []struct {
		name string
		xff  []string
		want []string
	}{
		{name: "none", want: []string{}},
		{name: "single", xff: []string{"a"}, want: []string{"a"}},
		{name: "list", xff: []string{"a, b ,c"}, want: []string{"c", "b", "a"}},
		{name: "values", xff: []string{"a,b", "c"}, want: []string{"c", "b", "a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := forwardedFromRight(tc.xff)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
				t.Fatalf("forwardedFromRight() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestForwardedFromRightCapped(t *testing.T) {
	values := make([]string, maxForwardedHops*2)
	for i := range values {
		values[i] = "a,b"
	}
	if got := len(forwardedFromRight(values)); got != maxForwardedHops {
		t.Fatalf("hops = %d, want %d", got, maxForwardedHops)
	}
}

func FuzzClientKey(f *testing.F) {
	f.Add("10.0.0.1:80", "198.51.100.1, 10.2.2.2")
	f.Add("[2001:db8::1]:443", "")
	f.Add("garbage", "1.2.3.4")
	f.Add("10.0.0.1:1", ",,,")
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8::/32")}
	f.Fuzz(func(t *testing.T, remote, xff string) {
		got, _ := ClientKey(remote, []string{xff}, trusted)
		if !got.IsValid() {
			t.Fatalf("invalid key for (%q, %q)", remote, xff)
		}
		if got != got.Masked() {
			t.Fatalf("key %s not masked", got)
		}
		if got.Addr().Zone() != "" {
			t.Fatalf("key %s retains zone", got)
		}
	})
}
