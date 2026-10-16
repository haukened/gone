package config

import (
	"strings"
	"testing"
)

// testMetricsToken is a token of exactly the minimum length.
const testMetricsToken = "0123456789abcdef"

func TestMetricsDisabledAllowsEmptyToken(t *testing.T) {
	cleanGoneEnvForTest(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MetricsAddr != "" || cfg.MetricsToken != "" {
		t.Fatalf("expected disabled metrics with empty token, got addr=%q token=%q", cfg.MetricsAddr, cfg.MetricsToken)
	}
}

func TestMetricsAddrWithoutTokenDisablesMetrics(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_METRICS_ADDR", "127.0.0.1:9090")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MetricsEnabled() {
		t.Fatalf("expected metrics disabled without token")
	}
}

func TestMetricsEnabled(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		token string
		want  bool
	}{
		{"neither", "", "", false},
		{"addr only", "127.0.0.1:9090", "", false},
		{"token only", "", "tok", false},
		{"both", "127.0.0.1:9090", "tok", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{MetricsAddr: tt.addr, MetricsToken: tt.token}
			if got := c.MetricsEnabled(); got != tt.want {
				t.Fatalf("MetricsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetricsAddrWithToken(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_METRICS_ADDR", "127.0.0.1:9090")
	t.Setenv("GONE_METRICS_TOKEN", testMetricsToken)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MetricsAddr != "127.0.0.1:9090" || cfg.MetricsToken != testMetricsToken || !cfg.MetricsEnabled() {
		t.Fatalf("metrics config mismatch: %+v", cfg)
	}
}

// TestMetricsTokenMinimumLength checks a short token still loads (metrics
// stay usable) but records one warning that names the fix and never echoes the
// token, while a 16-character token, a longer one and an empty one record none.
func TestMetricsTokenMinimumLength(t *testing.T) {
	cases := []struct {
		name, token string
		warn        bool
	}{
		{"empty leaves metrics off", "", false},
		{"one short", testMetricsToken[:15], true},
		{"placeholder", "change-me", true},
		{"minimum", testMetricsToken, false},
		{"generated", "q3JZ0m9v1c8wX5kLhT2uYp7eNf4aBd6s", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanGoneEnvForTest(t)
			t.Setenv("GONE_METRICS_ADDR", "127.0.0.1:9090")
			t.Setenv("GONE_METRICS_TOKEN", tc.token)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if !tc.warn {
				if len(cfg.Warnings) != 0 {
					t.Fatalf("unexpected warnings: %v", cfg.Warnings)
				}
				return
			}
			if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "GONE_METRICS_TOKEN") || !strings.Contains(cfg.Warnings[0], "openssl rand -base64 24") {
				t.Fatalf("Warnings = %v", cfg.Warnings)
			}
			if strings.Contains(cfg.Warnings[0], tc.token) {
				t.Fatalf("warning echoes the token: %v", cfg.Warnings)
			}
			if cfg.MetricsToken != tc.token || !cfg.MetricsEnabled() {
				t.Fatalf("short token should still enable metrics: %+v", cfg)
			}
		})
	}
}
