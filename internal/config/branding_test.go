package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseAccent(t *testing.T) {
	tests := []struct {
		name, raw, light, dark, wantErr string
	}{
		{name: "unset"},
		{name: "default violet passes", raw: "#4B3F8A/#cfc6f7", light: "#4b3f8a", dark: "#cfc6f7"},
		{name: "spaces", raw: " #1d4ed8 / #93c5fd ", light: "#1d4ed8", dark: "#93c5fd"},
		{name: "light too pale", raw: "#ffff00/#93c5fd", wantErr: "light-theme colour #ffff00"},
		{name: "dark too dark", raw: "#1d4ed8/#000080", wantErr: "dark-theme colour #000080"},
		{name: "one colour", raw: "#1d4ed8", wantErr: "#rrggbb/#rrggbb"},
		{name: "short hex", raw: "#123/#abc", wantErr: "#rrggbb/#rrggbb"},
		{name: "not hex", raw: "#12345g/#abcdef", wantErr: "#rrggbb/#rrggbb"},
		{name: "named colour", raw: "blue/white", wantErr: "#rrggbb/#rrggbb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			light, dark, err := parseAccent(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.HasPrefix(err.Error(), "GONE_BRAND_ACCENT:") {
					t.Fatalf("err = %v, want GONE_BRAND_ACCENT mentioning %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || light != tc.light || dark != tc.dark {
				t.Fatalf("parseAccent(%q) = %q, %q, %v", tc.raw, light, dark, err)
			}
		})
	}
}

func TestContrastRatio(t *testing.T) {
	if got := contrastRatio("#000000", "#ffffff"); got < 20.99 || got > 21.01 {
		t.Fatalf("black on white = %.3f, want 21", got)
	}
	if got := contrastRatio("#777777", "#777777"); got != 1 {
		t.Fatalf("same colour = %.3f, want 1", got)
	}
	// #767676 on white is the classic 4.54:1 grey.
	if got := contrastRatio("#767676", "#ffffff"); got < 4.5 || got > 4.6 {
		t.Fatalf("#767676 on white = %.3f, want about 4.54", got)
	}
}

func TestValidateLinkURL(t *testing.T) {
	tests := []struct {
		raw    string
		mailto bool
		ok     bool
	}{
		{"", false, true},
		{"https://example.com/imprint", false, true},
		{"https://example.com/a,b?q=1", false, true},
		{"mailto:security@example.com", true, true},
		{"mailto:security@example.com", false, false},
		{"http://example.com/", false, false},
		{"javascript:alert(1)", true, false},
		{"/privacy", false, false},
		{"https:///no-host", false, false},
		{"https://user:pw@example.com/", false, false},
		{"mailto:", true, false},
		{"https://example.com/" + strings.Repeat("a", maxLinkURLBytes), false, false},
	}
	for _, tc := range tests {
		err := validateLinkURL("GONE_PRIVACY_URL", tc.raw, tc.mailto)
		if (err == nil) != tc.ok {
			t.Errorf("validateLinkURL(%q, mailto=%v) = %v, want ok=%v", tc.raw, tc.mailto, err, tc.ok)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "GONE_PRIVACY_URL:") {
			t.Errorf("error %q does not name the variable", err)
		}
	}
}

func TestValidateBranding(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "none", mutate: func(*Config) {}},
		{name: "all set", mutate: func(c *Config) {
			c.BrandName, c.BrandAccent = "Acme, Inc. Secrets", "#1d4ed8/#93c5fd"
			c.ContactURL, c.ImprintURL, c.PrivacyURL = "mailto:it@example.com", "https://example.com/imprint", "https://example.com/privacy"
		}},
		{name: "name too long", mutate: func(c *Config) { c.BrandName = strings.Repeat("é", maxBrandNameRunes+1) }, wantErr: "GONE_BRAND_NAME"},
		{name: "name control", mutate: func(c *Config) { c.BrandName = "Acme\nSecrets" }, wantErr: "GONE_BRAND_NAME"},
		{name: "bad accent", mutate: func(c *Config) { c.BrandAccent = "#fff/#000" }, wantErr: "GONE_BRAND_ACCENT"},
		{name: "contact http", mutate: func(c *Config) { c.ContactURL = "http://example.com" }, wantErr: "GONE_CONTACT_URL"},
		{name: "imprint mailto", mutate: func(c *Config) { c.ImprintURL = "mailto:x@example.com" }, wantErr: "GONE_IMPRINT_URL"},
		{name: "privacy relative", mutate: func(c *Config) { c.PrivacyURL = "/privacy" }, wantErr: "GONE_PRIVACY_URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultAppConfig
			tc.mutate(&cfg)
			err := validateBranding(&cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateBranding: %v", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr+":") {
				t.Fatalf("err = %v, want %s", err, tc.wantErr)
			}
		})
	}
}

func TestLoadBrandingKeepsCommas(t *testing.T) {
	cleanGoneEnvForTest(t)
	t.Setenv("GONE_BRAND_NAME", "Acme, Inc.")
	t.Setenv("GONE_PRIVACY_URL", "https://example.com/a,b")
	t.Setenv("GONE_BRAND_ACCENT", "#1D4ED8/#93c5fd")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BrandName != "Acme, Inc." || cfg.PrivacyURL != "https://example.com/a,b" {
		t.Fatalf("name %q, privacy %q", cfg.BrandName, cfg.PrivacyURL)
	}
	if cfg.AccentLight != "#1d4ed8" || cfg.AccentDark != "#93c5fd" {
		t.Fatalf("accent %q/%q", cfg.AccentLight, cfg.AccentDark)
	}
}

// TestBackgroundsMatchTokens keeps the contrast backgrounds in step with
// web/css/tokens.css.
func TestBackgroundsMatchTokens(t *testing.T) {
	css, err := os.ReadFile("../../web/css/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--pearl: " + lightBackgrounds[0] + ";",
		"--dusk: " + darkBackgrounds[0] + ";",
		"--pane-solid: light-dark(" + lightBackgrounds[1] + ", " + darkBackgrounds[1] + ");",
	}
	for _, w := range want {
		if !strings.Contains(string(css), w) {
			t.Errorf("tokens.css lacks %q; update lightBackgrounds/darkBackgrounds", w)
		}
	}
}
