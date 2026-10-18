package config

import (
	"strings"
	"testing"
)

func TestLoadAuth(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_AUTH", "custom")
	t.Setenv("GONE_AUTH_HEADER", "X-Gone-User")
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), "GONE_TRUSTED_PROXIES: required with GONE_AUTH=custom") {
		t.Fatalf("custom without trusted proxies: %v", err)
	}
	t.Setenv("GONE_TRUSTED_PROXIES", "10.0.0.2")
	t.Setenv("GONE_AUTH_SIGNIN_URL", "https://auth.example.com/?rd=https://gone.example.com{return},x")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("custom with trusted proxies: %v", err)
	}
	if s := cfg.AuthSettings(); s.Provider != "custom" || s.Header != "X-Gone-User" || !strings.HasSuffix(s.SignInURL, ",x") {
		t.Fatalf("settings = %+v", s)
	}
}

func TestLoadAuthWarning(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_AUTH", "google-iap")
	t.Setenv("GONE_AUTH_AUDIENCE", "/projects/1/global/backendServices/2")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "GONE_AUTH=google-iap without GONE_TRUSTED_PROXIES") {
		t.Fatalf("warnings = %v", cfg.Warnings)
	}
}

func TestLoadAuthOffIsQuiet(t *testing.T) {
	cleanGoneEnvForTest(t)
	cfg, err := Load()
	if err != nil || len(cfg.Warnings) != 0 || cfg.AuthSettings().Enabled() {
		t.Fatalf("default: %v %v", err, cfg.Warnings)
	}
	t.Setenv("GONE_AUTH_TEAM", "acme")
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), "GONE_AUTH_TEAM: set without GONE_AUTH") {
		t.Fatalf("leftover option: %v", err)
	}
}

// TestLoadAuthKubernetesWarning checks a plain-header provider warns when
// Gone runs in a Kubernetes pod, and that signed providers and other hosts
// don't.
func TestLoadAuthKubernetesWarning(t *testing.T) {
	plain := func(t *testing.T) {
		t.Helper()
		cleanGoneEnvForTest(t)
		t.Setenv("GONE_AUTH", "authelia")
		t.Setenv("GONE_TRUSTED_PROXIES", "127.0.0.1")
	}
	kube := func(cfg *Config) bool {
		for _, w := range cfg.Warnings {
			if strings.HasPrefix(w, "GONE_AUTH=authelia on Kubernetes:") && strings.Contains(w, "docs/deploy/kubernetes.md#sign-in") {
				return true
			}
		}
		return false
	}

	plain(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	cfg, err := Load()
	if err != nil || !kube(cfg) {
		t.Fatalf("authelia on Kubernetes: %v %v", err, cfg.Warnings)
	}

	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	if cfg, err = Load(); err != nil || kube(cfg) || len(cfg.Warnings) != 0 {
		t.Fatalf("authelia elsewhere: %v %v", err, cfg.Warnings)
	}

	cleanGoneEnvForTest(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("GONE_AUTH", "google-iap")
	t.Setenv("GONE_AUTH_AUDIENCE", "/projects/1/global/backendServices/2")
	t.Setenv("GONE_TRUSTED_PROXIES", "10.244.0.0/16")
	if cfg, err = Load(); err != nil || len(cfg.Warnings) != 0 {
		t.Fatalf("signed provider on Kubernetes: %v %v", err, cfg.Warnings)
	}
}
