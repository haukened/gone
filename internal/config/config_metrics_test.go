package config

import "testing"

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
	t.Setenv("GONE_METRICS_TOKEN", "tok")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MetricsAddr != "127.0.0.1:9090" || cfg.MetricsToken != "tok" || !cfg.MetricsEnabled() {
		t.Fatalf("metrics config mismatch: %+v", cfg)
	}
}
