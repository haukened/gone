package config

import (
	"os"
	"testing"
)

// cleanEnvVars removes Gone environment variables and returns original values.
func cleanEnvVars(t *testing.T) map[string]string {
	t.Helper()
	orig := make(map[string]string)
	vars := []string{
		"GONE_ADDR",
		"GONE_DATA_DIR",
		"GONE_INLINE_MAX_BYTES",
		"GONE_MAX_BYTES",
		"GONE_TTL_OPTIONS",
		"GONE_CLAIM_LEASE",
		"GONE_METRICS_ADDR",
		"GONE_METRICS_TOKEN",
		"GONE_RATE_CREATE",
		"GONE_RATE_READ",
		"GONE_RATE_BURST",
		"GONE_TRUSTED_PROXIES",
		"GONE_REQUIRE_PASSPHRASE",
		"GONE_MAX_FILES",
		"GONE_BRAND_NAME",
		"GONE_BRAND_LOGO",
		"GONE_BRAND_ACCENT",
		"GONE_CONTACT_URL",
		"GONE_IMPRINT_URL",
		"GONE_PRIVACY_URL",
		"GONE_AUTH",
		"GONE_AUTH_TEAM",
		"GONE_AUTH_AUDIENCE",
		"GONE_AUTH_ISSUER",
		"GONE_AUTH_HOST",
		"GONE_AUTH_JWKS_URL",
		"GONE_AUTH_ALG",
		"GONE_AUTH_HEADER",
		"GONE_AUTH_FORMAT",
		"GONE_AUTH_VALUE",
		"GONE_AUTH_SIGNIN_URL",
	}
	for _, v := range vars {
		if val := os.Getenv(v); val != "" {
			orig[v] = val
		}
		if err := os.Unsetenv(v); err != nil {
			t.Fatalf("unsetenv %q: %v", v, err)
		}
	}
	return orig
}

// restoreEnvVars restores Gone environment variables saved by cleanEnvVars.
func restoreEnvVars(t *testing.T, orig map[string]string) {
	t.Helper()
	for k, v := range orig {
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("setenv %q: %v", k, err)
		}
	}
}

// cleanGoneEnvForTest cleans Gone variables and schedules their restoration.
func cleanGoneEnvForTest(t *testing.T) {
	t.Helper()
	orig := cleanEnvVars(t)
	t.Cleanup(func() { restoreEnvVars(t, orig) })
}

// setConfigTestEnv sets each provided environment variable for a test.
func setConfigTestEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
}
