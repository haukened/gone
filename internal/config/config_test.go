package config

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	assert.EqualValues(t, DefaultAppConfig, *cfg)
}

func TestValidPaths(t *testing.T) {
	valid := []string{
		"data",
		"/var/lib/gone",
		"./data",
		"relative/path/to/data",
		"nested/dir/structure",
	}
	for _, p := range valid {
		t.Setenv("GONE_DATA_DIR", p)
		cfg, err := Load()
		if err != nil {
			t.Errorf("expected valid path %q, got error: %v", p, err)
			continue
		}
		if cfg.DataDir != p {
			t.Errorf("expected DataDir %q, got %q", p, cfg.DataDir)
		}
	}
}

func TestInvalidPaths(t *testing.T) {
	invalid := []string{
		"",
		".",
		"/",
		"//",
		"../data",
		"data/..",
		"data/../../../etc",
	}
	for _, p := range invalid {
		t.Setenv("GONE_DATA_DIR", p)
		_, err := Load()
		if err == nil {
			t.Errorf("expected error for invalid path %q, got nil", p)
			continue
		}
	}
}

func TestValidIPPort(t *testing.T) {
	type sample struct {
		Addr string `validate:"ip_port"`
	}

	v := validator.New()
	if err := v.RegisterValidation("ip_port", validIPPort); err != nil {
		t.Fatalf("register validation: %v", err)
	}

	tests := []struct {
		name  string
		addr  string
		valid bool
	}{
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sample{Addr: tc.addr}
			err := v.Struct(&s)
			if tc.valid && err != nil {
				t.Fatalf("expected valid, got error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestSQLiteDSN(t *testing.T) {
	params := "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_synchronous=FULL"

	join := func(a, b string) string {
		if len(a) == 0 {
			return b
		}
		if a[len(a)-1] == '/' {
			return a + b
		}
		return a + "/" + b
	}

	countRune := func(s string, r rune) int {
		c := 0
		for _, ch := range s {
			if ch == r {
				c++
			}
		}
		return c
	}

	contains := func(haystack, needle string) bool {
	outer:
		for i := 0; i+len(needle) <= len(haystack); i++ {
			for j := 0; j < len(needle); j++ {
				if haystack[i+j] != needle[j] {
					continue outer
				}
			}
			return true
		}
		return false
	}

	type tc struct {
		name    string
		dataDir string
	}
	tests := []tc{
		{name: "default_config", dataDir: DefaultAppConfig.DataDir},
		{name: "relative_no_slash", dataDir: "data"},
		{name: "relative_trailing_slash", dataDir: "data/"},
		{name: "absolute_no_slash", dataDir: "/var/lib/gone"},
		{name: "absolute_trailing_slash", dataDir: "/var/lib/gone/"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Addr:     ":8080",
				DataDir:  tt.dataDir,
				MaxBytes: DefaultAppConfig.MaxBytes,
				MinTTL:   DefaultAppConfig.MinTTL,
				MaxTTL:   DefaultAppConfig.MaxTTL,
			}

			got := c.SQLiteDSN()
			wantPath := join(tt.dataDir, "gone.db")
			want := "file:" + wantPath + params

			assert.Equal(t, want, got, "expected DSN mismatch")

			// Structural assertions.
			assert.True(t, contains(got, "_journal_mode=WAL"), "missing WAL mode")
			assert.True(t, contains(got, "_foreign_keys=on"), "missing foreign keys pragma")
			assert.True(t, contains(got, "_busy_timeout=5000"), "missing busy timeout")
			assert.True(t, contains(got, "_synchronous=FULL"), "missing synchronous FULL")
			assert.Equal(t, 1, countRune(got, '?'), "expected exactly one '?' in DSN")
		})
	}
}

func TestLoadDefaultError(t *testing.T) {
	// swap out the defaultLoader to return an error
	orig := defaultLoader
	t.Cleanup(func() { defaultLoader = orig })
	defaultLoader = func(k *koanf.Koanf) error {
		assert.NotNil(t, k)
		return assert.AnError
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, assert.AnError) {
		t.Fatalf("expected assert.AnError, got: %v", err)
	}
}

func TestLoadEnvError(t *testing.T) {
	// swap out the envLoader to return an error
	orig := envLoader
	t.Cleanup(func() { envLoader = orig })
	envLoader = func(k *koanf.Koanf) error {
		assert.NotNil(t, k)
		return assert.AnError
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, assert.AnError) {
		t.Fatalf("expected assert.AnError, got: %v", err)
	}
}

func TestRegisterValidationFails(t *testing.T) {
	orig := registerValidators
	t.Cleanup(func() { registerValidators = orig })
	registerValidators = func(v *validator.Validate) error {
		assert.NotNil(t, v)
		return assert.AnError
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, assert.AnError) {
		t.Fatalf("expected assert.AnError, got: %v", err)
	}
}

func TestBadTTL(t *testing.T) {
	t.Setenv("GONE_MIN_TTL", "10m")
	t.Setenv("GONE_MAX_TTL", "5m") // less than min
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "min_ttl must be less than max_ttl" {
		t.Fatalf("expected min/max ttl error, got: %v", err)
	}
}
