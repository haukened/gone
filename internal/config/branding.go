package config

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxBrandNameRunes caps GONE_BRAND_NAME so it fits the header.
	maxBrandNameRunes = 40
	// maxLinkURLBytes caps the footer link URLs.
	maxLinkURLBytes = 2048
)

// scalarKeys are settings whose values may contain commas and are never
// split into lists by envLoader.
var scalarKeys = map[string]bool{
	"brand_name": true, "brand_logo": true, "brand_accent": true,
	"contact_url": true, "imprint_url": true, "privacy_url": true,
}

// validateBranding checks the operator branding settings and derives the
// light and dark accent colours.
//
// Parameters:
//   - cfg: the configuration to check; AccentLight and AccentDark are set.
//
// Returns:
//   - error: the first invalid setting, named by its environment variable.
func validateBranding(cfg *Config) error {
	if err := validateBrandName(cfg.BrandName); err != nil {
		return err
	}
	light, dark, err := parseAccent(cfg.BrandAccent)
	if err != nil {
		return err
	}
	cfg.AccentLight, cfg.AccentDark = light, dark
	links := []struct {
		name, value string
		mailto      bool
	}{
		{"GONE_CONTACT_URL", cfg.ContactURL, true},
		{"GONE_IMPRINT_URL", cfg.ImprintURL, false},
		{"GONE_PRIVACY_URL", cfg.PrivacyURL, false},
	}
	for _, l := range links {
		if err := validateLinkURL(l.name, l.value, l.mailto); err != nil {
			return err
		}
	}
	return nil
}

// validateBrandName checks GONE_BRAND_NAME: empty, or 1 to 40 characters
// of text with no control characters.
//
// Parameters:
//   - name: the trimmed setting.
//
// Returns:
//   - error: naming GONE_BRAND_NAME when invalid.
func validateBrandName(name string) error {
	if name == "" {
		return nil
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxBrandNameRunes {
		return fmt.Errorf("GONE_BRAND_NAME: must be at most %d characters of UTF-8 text", maxBrandNameRunes)
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("GONE_BRAND_NAME: must not contain control characters")
	}
	return nil
}

// parseAccent parses GONE_BRAND_ACCENT, "#light/#dark", and checks each
// colour's contrast against its theme's backgrounds.
//
// Parameters:
//   - raw: the setting; empty means no accent.
//
// Returns:
//   - string, string: the light and dark colours in lowercase, or "" when unset.
//   - error: naming GONE_BRAND_ACCENT when malformed or below the contrast minimum.
func parseAccent(raw string) (string, string, error) {
	if raw == "" {
		return "", "", nil
	}
	light, dark, ok := strings.Cut(raw, "/")
	light, dark = strings.ToLower(strings.TrimSpace(light)), strings.ToLower(strings.TrimSpace(dark))
	if !ok || !isHexColor(light) || !isHexColor(dark) {
		return "", "", fmt.Errorf("GONE_BRAND_ACCENT: want two colours as #rrggbb/#rrggbb (light theme, then dark), got %q", raw)
	}
	if err := checkContrast("light", light, lightBackgrounds); err != nil {
		return "", "", err
	}
	if err := checkContrast("dark", dark, darkBackgrounds); err != nil {
		return "", "", err
	}
	return light, dark, nil
}

// validateLinkURL checks a footer link: empty, an absolute https URL with a
// host, or (when mailto is allowed) a mailto: address.
//
// Parameters:
//   - name: the environment variable, for the message.
//   - raw: the trimmed setting.
//   - mailto: whether mailto: links are allowed.
//
// Returns:
//   - error: naming the variable when invalid.
func validateLinkURL(name, raw string, mailto bool) error {
	if raw == "" {
		return nil
	}
	want := "an https:// URL"
	if mailto {
		want = "an https:// URL or a mailto: address"
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxLinkURLBytes {
		return fmt.Errorf("%s: must be %s of at most %d bytes", name, want, maxLinkURLBytes)
	}
	if !linkAllowed(u, mailto) {
		return fmt.Errorf("%s: must be %s, got %q", name, want, raw)
	}
	return nil
}

// linkAllowed reports whether u is an https URL with a host and no user
// info, or (when mailto is allowed) a mailto: address.
//
// Parameters:
//   - u: the parsed link.
//   - mailto: whether mailto: links are allowed.
//
// Returns:
//   - bool: true when the link may be shown.
func linkAllowed(u *url.URL, mailto bool) bool {
	if u.Scheme == "https" {
		return u.Host != "" && u.User == nil
	}
	return mailto && u.Scheme == "mailto" && u.Opaque != ""
}
