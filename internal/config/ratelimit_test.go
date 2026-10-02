package config

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/ratelimit"
)

func TestRateLimitEnv(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantCreate ratelimit.Rate
		wantRead   ratelimit.Rate
		wantBurst  int
		wantProxy  []string
		wantErr    string
	}{
		{
			name:       "defaults",
			wantCreate: ratelimit.Rate{Count: 10, Per: time.Minute},
			wantRead:   ratelimit.Rate{Count: 30, Per: time.Minute},
			wantBurst:  10,
		},
		{
			name:       "overrides",
			env:        map[string]string{"GONE_RATE_CREATE": "5/s", "GONE_RATE_READ": "100/h", "GONE_RATE_BURST": "3"},
			wantCreate: ratelimit.Rate{Count: 5, Per: time.Second},
			wantRead:   ratelimit.Rate{Count: 100, Per: time.Hour},
			wantBurst:  3,
		},
		{
			name:      "disabled",
			env:       map[string]string{"GONE_RATE_CREATE": "0", "GONE_RATE_READ": "0"},
			wantBurst: 10,
		},
		{
			name:       "single proxy",
			env:        map[string]string{"GONE_TRUSTED_PROXIES": "10.0.0.0/8"},
			wantCreate: ratelimit.Rate{Count: 10, Per: time.Minute},
			wantRead:   ratelimit.Rate{Count: 30, Per: time.Minute},
			wantBurst:  10,
			wantProxy:  []string{"10.0.0.0/8"},
		},
		{
			name:       "proxy list",
			env:        map[string]string{"GONE_TRUSTED_PROXIES": "10.1.2.3/8, 192.0.2.1,,2001:db8::/48"},
			wantCreate: ratelimit.Rate{Count: 10, Per: time.Minute},
			wantRead:   ratelimit.Rate{Count: 30, Per: time.Minute},
			wantBurst:  10,
			wantProxy:  []string{"10.0.0.0/8", "192.0.2.1/32", "2001:db8::/48"},
		},
		{
			name:       "empty proxy list",
			env:        map[string]string{"GONE_TRUSTED_PROXIES": ""},
			wantCreate: ratelimit.Rate{Count: 10, Per: time.Minute},
			wantRead:   ratelimit.Rate{Count: 30, Per: time.Minute},
			wantBurst:  10,
		},
		{name: "bad create", env: map[string]string{"GONE_RATE_CREATE": "fast"}, wantErr: "GONE_RATE_CREATE"},
		{name: "bad read", env: map[string]string{"GONE_RATE_READ": "-1/m"}, wantErr: "GONE_RATE_READ"},
		{name: "bad proxy", env: map[string]string{"GONE_TRUSTED_PROXIES": "0.0.0.0/0"}, wantErr: "GONE_TRUSTED_PROXIES"},
		{name: "burst zero", env: map[string]string{"GONE_RATE_BURST": "0"}, wantErr: "RateBurst"},
		{name: "burst too large", env: map[string]string{"GONE_RATE_BURST": "1001"}, wantErr: "RateBurst"},
		{name: "burst garbage", env: map[string]string{"GONE_RATE_BURST": "many"}, wantErr: "rate_burst"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := cleanEnvVars(t)
			t.Cleanup(func() { restoreEnvVars(t, orig) })
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Load() err = %v, want mention of %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.CreateRate != tc.wantCreate || cfg.ReadRate != tc.wantRead || cfg.RateBurst != tc.wantBurst {
				t.Fatalf("got create=%+v read=%+v burst=%d", cfg.CreateRate, cfg.ReadRate, cfg.RateBurst)
			}
			if got := prefixStrings(cfg.TrustedPrefixes); got != strings.Join(tc.wantProxy, ",") {
				t.Fatalf("TrustedPrefixes = %s, want %v", got, tc.wantProxy)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    string
		wantErr bool
	}{
		{name: "nil", in: nil, want: ""},
		{name: "blanks only", in: []string{"", "  "}, want: ""},
		{name: "ipv4 host", in: []string{"192.0.2.1"}, want: "192.0.2.1/32"},
		{name: "ipv6 host", in: []string{"2001:db8::1"}, want: "2001:db8::1/128"},
		{name: "ipv6 host with zone", in: []string{"fe80::1%eth0"}, want: "fe80::1/128"},
		{name: "masked", in: []string{"10.20.30.40/16"}, want: "10.20.0.0/16"},
		{name: "ipv4 /8 allowed", in: []string{"10.0.0.0/8"}, want: "10.0.0.0/8"},
		{name: "ipv6 /32 allowed", in: []string{"2001:db8::/32"}, want: "2001:db8::/32"},
		{name: "trimmed", in: []string{" 192.0.2.0/24 "}, want: "192.0.2.0/24"},
		{name: "ipv4 too broad", in: []string{"10.0.0.0/7"}, wantErr: true},
		{name: "ipv4 any", in: []string{"0.0.0.0/0"}, wantErr: true},
		{name: "ipv6 too broad", in: []string{"2001::/31"}, wantErr: true},
		{name: "ipv6 any", in: []string{"::/0"}, wantErr: true},
		{name: "ipv4-mapped prefix", in: []string{"::ffff:10.0.0.0/104"}, wantErr: true},
		{name: "ipv4-mapped host", in: []string{"::ffff:10.0.0.1"}, wantErr: true},
		{name: "garbage", in: []string{"proxy.local"}, wantErr: true},
		{name: "bad bits", in: []string{"10.0.0.0/33"}, wantErr: true},
		{name: "one bad entry fails all", in: []string{"10.0.0.0/8", "nope"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTrustedProxies(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseTrustedProxies(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTrustedProxies(%q) error: %v", tc.in, err)
			}
			if s := prefixStrings(got); s != tc.want {
				t.Fatalf("parseTrustedProxies(%q) = %s, want %s", tc.in, s, tc.want)
			}
		})
	}
}

func TestDeriveRateLimitsWrapsErrors(t *testing.T) {
	cfg := Config{RateCreate: "1/x", RateRead: "1/m"}
	err := deriveRateLimits(&cfg)
	if !errors.Is(err, ratelimit.ErrInvalidRate) {
		t.Fatalf("err = %v, want wrapped ErrInvalidRate", err)
	}
}

// prefixStrings joins prefixes with commas for comparison.
func prefixStrings(ps []netip.Prefix) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return strings.Join(parts, ",")
}
