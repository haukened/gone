package brand

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/config"
)

const testSVG = `<?xml version="1.0"?><!-- logo --><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`

// writeFile writes data to a file in a temporary directory.
func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadNone(t *testing.T) {
	b, err := Load(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Branded() || b.LogoURL() != "" || b.AccentURL() != "" || len(b.Links) != 0 {
		t.Fatalf("empty config gave %+v", b)
	}
	var nilBrand *Brand
	if nilBrand.Branded() || nilBrand.LogoURL() != "" || nilBrand.AccentURL() != "" {
		t.Fatal("nil Brand must report no branding")
	}
}

func TestLoadAll(t *testing.T) {
	cfg := &config.Config{
		BrandName: "Acme", BrandLogo: writeFile(t, "logo.svg", []byte(testSVG)),
		AccentLight: "#1d4ed8", AccentDark: "#93c5fd",
		ContactURL: "mailto:it@example.com", PrivacyURL: "https://example.com/privacy",
	}
	b, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Branded() || b.Name != "Acme" || b.Logo.Type != "image/svg+xml" {
		t.Fatalf("brand = %+v", b)
	}
	if !strings.HasPrefix(b.LogoURL(), LogoPath+"?v=") || len(b.Logo.Version) != 12 {
		t.Fatalf("logo URL %q", b.LogoURL())
	}
	css := string(b.Accent.Data)
	if !strings.Contains(css, "--color-accent: light-dark(#1d4ed8, #93c5fd);") || !strings.Contains(css, "40%, transparent") || b.Accent.Type != "text/css; charset=utf-8" {
		t.Fatalf("accent css:\n%s", css)
	}
	if want := []Link{{"contact", "mailto:it@example.com", true}, {"privacy", "https://example.com/privacy", false}}; len(b.Links) != 2 || b.Links[0] != want[0] || b.Links[1] != want[1] {
		t.Fatalf("links = %+v", b.Links)
	}
	again, _ := Load(cfg)
	if again.AccentURL() != b.AccentURL() || again.LogoURL() != b.LogoURL() {
		t.Fatal("asset versions must be stable")
	}
}

func TestLinksAloneAreNotBranded(t *testing.T) {
	b, err := Load(&config.Config{ImprintURL: "https://example.com/imprint"})
	if err != nil || b.Branded() || len(b.Links) != 1 {
		t.Fatalf("brand = %+v, err %v", b, err)
	}
}

func TestLogoTypes(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	tests := []struct {
		name    string
		data    []byte
		typ     string
		wantErr string
	}{
		{"png", png, "image/png", ""},
		{"svg", []byte(testSVG), "image/svg+xml", ""},
		{"svg no namespace", []byte(`<svg viewBox="0 0 1 1"/>`), "", "root element must be <svg"},
		{"html", []byte(`<html xmlns="http://www.w3.org/1999/xhtml"><body/></html>`), "", "root element must be <svg"},
		{"text", []byte("hello"), "", "neither a PNG nor an SVG"},
		{"empty", nil, "", "neither a PNG nor an SVG"},
		{"jpeg", []byte("\xff\xd8\xff\xe0JFIF"), "", "neither a PNG nor an SVG"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Load(&config.Config{BrandLogo: writeFile(t, "logo", tc.data)})
			if tc.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), "GONE_BRAND_LOGO: ") || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || b.Logo.Type != tc.typ {
				t.Fatalf("type %v, err %v", b, err)
			}
		})
	}
}

func TestLogoFileErrors(t *testing.T) {
	big := writeFile(t, "big.svg", append([]byte(testSVG), make([]byte, MaxLogoBytes)...))
	missing := filepath.Join(t.TempDir(), "nope.svg")
	for path, want := range map[string]string{big: "larger than 256 KiB", missing: "cannot read " + missing + ": no such file"} {
		_, err := Load(&config.Config{BrandLogo: path})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load(%s) err = %v, want %q", filepath.Base(path), err, want)
		}
	}
}

func TestLogoDirectory(t *testing.T) {
	_, err := Load(&config.Config{BrandLogo: t.TempDir()})
	if err == nil || !strings.HasPrefix(err.Error(), "GONE_BRAND_LOGO: cannot read") {
		t.Fatalf("err = %v", err)
	}
}
