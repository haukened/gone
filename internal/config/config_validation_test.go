package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidIPPort(t *testing.T) {
	for _, tc := range ipPortValidationCases() {
		t.Run(tc.name, func(t *testing.T) {
			if got := validIPPort(tc.addr); got != tc.valid {
				t.Fatalf("validIPPort(%q) = %v, want %v", tc.addr, got, tc.valid)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range validateCases() {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultAppConfig
			tc.mutate(&cfg)
			err := validate(&cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate() err = %v, want mention of %q", err, tc.wantErr)
			}
		})
	}
}

type validateCase struct {
	name    string
	mutate  func(*Config)
	wantErr string
}

// validateCases returns one passing fixture and one failing fixture per rule.
func validateCases() []validateCase {
	return []validateCase{
		{name: "defaults", mutate: func(*Config) {}},
		{name: "metrics addr", mutate: func(c *Config) { c.MetricsAddr = "127.0.0.1:9090" }},
		{name: "lease at max", mutate: func(c *Config) { c.ClaimLease = maxClaimLease }},
		{name: "bad addr", mutate: func(c *Config) { c.Addr = "localhost:8080" }, wantErr: "GONE_ADDR"},
		{name: "empty addr", mutate: func(c *Config) { c.Addr = "" }, wantErr: "GONE_ADDR"},
		{name: "bad metrics addr", mutate: func(c *Config) { c.MetricsAddr = "9090" }, wantErr: "GONE_METRICS_ADDR"},
		{name: "bad data dir", mutate: func(c *Config) { c.DataDir = "../data" }, wantErr: "GONE_DATA_DIR"},
		{name: "zero inline max", mutate: func(c *Config) { c.InlineMaxBytes = 0 }, wantErr: "GONE_INLINE_MAX_BYTES"},
		{name: "negative max", mutate: func(c *Config) { c.MaxBytes = -1 }, wantErr: "GONE_MAX_BYTES"},
		{name: "no ttl options", mutate: func(c *Config) { c.TTLOptions = nil }, wantErr: "GONE_TTL_OPTIONS"},
		{name: "single ttl", mutate: func(c *Config) { c.MinTTL, c.MaxTTL = time.Hour, time.Hour }, wantErr: "GONE_TTL_OPTIONS"},
		{name: "zero min ttl", mutate: func(c *Config) { c.MinTTL = 0 }, wantErr: "GONE_TTL_OPTIONS"},
		{name: "zero lease", mutate: func(c *Config) { c.ClaimLease = 0 }, wantErr: "GONE_CLAIM_LEASE"},
		{name: "long lease", mutate: func(c *Config) { c.ClaimLease = maxClaimLease + time.Second }, wantErr: "GONE_CLAIM_LEASE"},
		{name: "burst zero", mutate: func(c *Config) { c.RateBurst = 0 }, wantErr: "GONE_RATE_BURST"},
		{name: "burst too large", mutate: func(c *Config) { c.RateBurst = 1001 }, wantErr: "GONE_RATE_BURST"},
	}
}

type ipPortValidationCase struct {
	name  string
	addr  string
	valid bool
}

// ipPortValidationCases returns IP:port validation fixtures.
func ipPortValidationCases() []ipPortValidationCase {
	return []ipPortValidationCase{
		{name: "empty", addr: "", valid: false},
		{name: "missing_port", addr: "127.0.0.1", valid: false},
		{name: "missing_port_after_colon", addr: "127.0.0.1:", valid: false},
		{name: "just_colon_port", addr: ":8080", valid: true},
		{name: "loopback_ipv4", addr: "127.0.0.1:8080", valid: true},
		{name: "any_ipv4_low_port", addr: "0.0.0.0:1", valid: true},
		{name: "ipv6_loopback", addr: "[::1]:8080", valid: true},
		{name: "ipv6_any", addr: "[::]:443", valid: true},
		{name: "unbracketed_ipv6", addr: "::1:8080", valid: false},
		{name: "hostname_not_ip", addr: "localhost:8080", valid: false},
		{name: "invalid_host_chars", addr: "not_an_ip!:80", valid: false},
		{name: "non_numeric_port", addr: "127.0.0.1:http", valid: false},
		{name: "port_zero", addr: "127.0.0.1:0", valid: false},
		{name: "port_max_valid", addr: "127.0.0.1:65535", valid: true},
		{name: "port_overflow", addr: "127.0.0.1:65536", valid: false},
		{name: "negative_port", addr: "127.0.0.1:-1", valid: false},
		{name: "multi_leading_zero_port", addr: "127.0.0.1:00080", valid: true},
		{name: "space_prefixed", addr: " :8080", valid: false},
		{name: "trailing_space", addr: "127.0.0.1:8080 ", valid: false},
		{name: "embedded_space", addr: "127.0. 0.1:8080", valid: false},
	}
}
