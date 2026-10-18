package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/config"
)

// testBrandName is the instance name the branded page tests use.
const testBrandName = "Acme Secrets"

var brandRef = regexp.MustCompile(`\s(?:src|href)="(/brand/[^"]+)"`)

// brandAll returns a config hook that sets every branding setting, with an
// SVG logo in a temporary file.
func brandAll(t *testing.T) func(*config.Config) {
	t.Helper()
	logo := filepath.Join(t.TempDir(), "logo.svg")
	if err := os.WriteFile(logo, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 1"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	return func(c *config.Config) {
		c.BrandName, c.BrandLogo, c.AccentLight, c.AccentDark = testBrandName, logo, "#1d4ed8", "#93c5fd"
		c.ContactURL, c.ImprintURL, c.PrivacyURL = "mailto:it@example.com", "https://example.com/imprint", "https://example.com/privacy"
	}
}

// TestPages_BrandedContract renders every page with full branding and runs
// the same DOM contract as the default pages: no inline code, declared ids
// and references, fingerprinted assets.
func TestPages_BrandedContract(t *testing.T) {
	h := pageRouterWith(t, brandAll(t))
	assets := map[string]bool{}
	for _, p := range pageContractCases {
		t.Run(p.path, func(t *testing.T) {
			checkPageContract(t, h, p, assets)
			_, body := get(h, p.path)
			refs := brandRef.FindAllStringSubmatch(body, -1)
			if len(refs) != 2 {
				t.Fatalf("want the logo and accent stylesheet, got %v", refs)
			}
			for _, m := range refs {
				if !strings.Contains(m[1], "?v=") {
					t.Errorf("%s has no fingerprint", m[1])
				}
				if code, _ := get(h, m[1]); code != http.StatusOK {
					t.Errorf("%s: status %d", m[1], code)
				}
			}
		})
	}
	checkReferencedAssets(t, h, assets)
}

// TestPages_BrandedHeaderFooter checks what a branded page shows.
func TestPages_BrandedHeaderFooter(t *testing.T) {
	_, body := get(pageRouterWith(t, brandAll(t)), "/")
	want := []string{
		`<a class="brand brand-operator" href="/" aria-label="Acme Secrets, home"`,
		`<img class="brand-logo" src="/brand/logo?v=`,
		`<span class="brand-name">Acme Secrets</span>`,
		`<link rel="stylesheet" href="/brand/accent.css?v=`,
		`<a href="mailto:it@example.com" data-i18n="common.footerContact">Contact</a>`,
		`<a href="https://example.com/imprint" rel="noopener noreferrer" data-i18n="common.footerImprint">Imprint</a>`,
		`<a href="https://example.com/privacy" rel="noopener noreferrer" data-i18n="common.footerPrivacy">Privacy</a>`,
		`>Powered by Gone v0.0.0-test</a>`,
	}
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("missing %s", w)
		}
	}
	if strings.Contains(body, `class="brand-g"`) {
		t.Error("the Gone wordmark is still in a branded header")
	}
}

// TestPages_BrandVariants covers a logo without a name, an accent alone
// (which keeps the Gone wordmark but says "Powered by Gone"), and footer
// links without branding, which keep the Gone wordmark and footer.
func TestPages_BrandVariants(t *testing.T) {
	all := brandAll(t)
	logoOnly := func(c *config.Config) { all(c); c.BrandName, c.AccentLight, c.AccentDark = "", "", "" }
	_, body := get(pageRouterWith(t, logoOnly), "/")
	for _, w := range []string{`aria-label="Home"`, `<img class="brand-logo"`, `Powered by Gone`} {
		if !strings.Contains(body, w) {
			t.Errorf("logo only: missing %s", w)
		}
	}
	if strings.Contains(body, `brand-name`) || strings.Contains(body, `/brand/accent.css`) {
		t.Error("logo only: unexpected name or accent")
	}

	accentOnly := func(c *config.Config) { c.AccentLight, c.AccentDark = "#1d4ed8", "#93c5fd" }
	_, body = get(pageRouterWith(t, accentOnly), "/")
	for _, w := range []string{`<span class="brand-g"></span>one</a>`, `/brand/accent.css?v=`, `>Powered by Gone v0.0.0-test</a>`} {
		if !strings.Contains(body, w) {
			t.Errorf("accent only: missing %s", w)
		}
	}
	if strings.Contains(body, `brand-operator`) {
		t.Error("accent only: the Gone wordmark was replaced")
	}

	linksOnly := func(c *config.Config) { c.ImprintURL = "https://example.com/imprint" }
	_, body = get(pageRouterWith(t, linksOnly), "/")
	if !strings.Contains(body, `class="brand-g"`) || !strings.Contains(body, `>Gone v0.0.0-test</a>`) || strings.Contains(body, ">Powered by") {
		t.Error("footer links alone must keep the Gone header and footer")
	}
	if !strings.Contains(body, `data-i18n="common.footerImprint"`) {
		t.Error("links only: missing the imprint link")
	}
}

// TestPages_DefaultUnbranded checks an instance without branding renders
// no branding at all. ("Powered by" itself is always in the page's embedded
// catalog, so the checks look for the rendered link text.)
func TestPages_DefaultUnbranded(t *testing.T) {
	_, body := get(pageRouter(t), "/")
	if !strings.Contains(body, `<span class="brand-g"></span>one</a>`) || strings.Contains(body, "/brand/") || strings.Contains(body, ">Powered by") {
		t.Error("default page shows branding")
	}
}
