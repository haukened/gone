package config

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

func TestDefaultConfig(t *testing.T) {
	cleanGoneEnvForTest(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !reflect.DeepEqual(DefaultAppConfig, *cfg) {
		t.Fatalf("Load() = %+v, want %+v", *cfg, DefaultAppConfig)
	}
	if cfg.ClaimLease != 2*time.Minute {
		t.Fatalf("ClaimLease = %v, want 2m", cfg.ClaimLease)
	}
	if cfg.MaxBytes != 10*1024*1024 {
		t.Fatalf("MaxBytes = %d, want 10 MiB", cfg.MaxBytes)
	}
}

func TestClaimLeaseConfig(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "ninety seconds", value: "90s", want: 90 * time.Second},
		{name: "zero rejected", value: "0s", wantErr: true},
		{name: "over max rejected", value: "16m", wantErr: true},
		{name: "garbage rejected", value: "garbage", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanGoneEnvForTest(t)
			t.Setenv("GONE_CLAIM_LEASE", tc.value)
			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for claim lease %q", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.ClaimLease != tc.want {
				t.Fatalf("ClaimLease = %v, want %v", cfg.ClaimLease, tc.want)
			}
		})
	}
}

func TestLoadPolicy(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_REQUIRE_PASSPHRASE", "true")
	t.Setenv("GONE_MAX_FILES", "0")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.RequirePassphrase || cfg.MaxFiles != 0 {
		t.Fatalf("policy = %v/%d, want true/0", cfg.RequirePassphrase, cfg.MaxFiles)
	}
	t.Setenv("GONE_MAX_FILES", "11")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "GONE_MAX_FILES") {
		t.Fatalf("Load() err = %v, want GONE_MAX_FILES", err)
	}
}

func TestLoadEnvList(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_TTL_OPTIONS", "5m,30m,1h")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	expected := []domain.TTLOption{
		{Duration: 5 * time.Minute, Label: "5m"},
		{Duration: 30 * time.Minute, Label: "30m"},
		{Duration: 1 * time.Hour, Label: "1h"},
	}
	if !reflect.DeepEqual(expected, cfg.TTLOptions) {
		t.Fatalf("TTL options = %+v, want %+v", cfg.TTLOptions, expected)
	}
}

func TestNoTTLOptions(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_TTL_OPTIONS", "")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestBadTTLOptions(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_TTL_OPTIONS", "invalid")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNumericEnvCoercion(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_MAX_BYTES", "2097152")
	t.Setenv("GONE_INLINE_MAX_BYTES", "4096")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MaxBytes != 2097152 {
		t.Fatalf("expected MaxBytes 2097152 got %d", cfg.MaxBytes)
	}
	if cfg.InlineMaxBytes != 4096 {
		t.Fatalf("expected InlineMaxBytes 4096 got %d", cfg.InlineMaxBytes)
	}
}
