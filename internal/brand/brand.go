// Package brand holds an operator's branding: the instance name, logo and
// accent colour shown in the header, and the contact, imprint and privacy
// links in the footer. Everything is read and checked once at startup and
// served from memory, so pages never load anything from another origin.
package brand

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/haukened/gone/v3/internal/config"
)

// MaxLogoBytes caps the logo file read from GONE_BRAND_LOGO.
const MaxLogoBytes = 256 << 10

// Paths the server serves the branding assets at.
const (
	LogoPath   = "/brand/logo"
	AccentPath = "/brand/accent.css"
)

// svgNamespace is the namespace an SVG must declare to render in <img>.
const svgNamespace = "http://www.w3.org/2000/svg"

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// Asset is a file served from memory.
type Asset struct {
	Data    []byte
	Type    string // Content-Type
	Version string // first 12 hex digits of the SHA-256, for ?v= cache busting
}

// URL returns the asset's fingerprinted URL under path.
//
// Parameters:
//   - path: LogoPath or AccentPath.
//
// Returns the URL with its ?v= version.
func (a *Asset) URL(path string) string {
	return path + "?v=" + a.Version
}

// Link is one footer link. Kind is "contact", "imprint" or "privacy".
type Link struct {
	Kind   string
	URL    string
	Mailto bool
}

// Brand is an operator's branding. The zero value means none.
type Brand struct {
	Name   string
	Logo   *Asset
	Accent *Asset
	Links  []Link
}

// Branded reports whether the instance is rebranded: a name, logo or accent
// is set, so the footer says "Powered by Gone". Footer links alone don't
// count; the footer then still says Gone.
//
// Returns true when the instance shows the operator's branding.
func (b *Brand) Branded() bool {
	return b != nil && (b.Name != "" || b.Logo != nil || b.Accent != nil)
}

// HasMark reports whether the operator's name or logo replaces the Gone
// wordmark in the header. An accent alone recolours the page but keeps the
// wordmark.
//
// Returns true when the header shows the operator's mark.
func (b *Brand) HasMark() bool {
	return b != nil && (b.Name != "" || b.Logo != nil)
}

// LogoURL returns the fingerprinted logo URL, or "" without a logo.
//
// Returns the URL.
func (b *Brand) LogoURL() string {
	if b == nil || b.Logo == nil {
		return ""
	}
	return b.Logo.URL(LogoPath)
}

// AccentURL returns the fingerprinted accent stylesheet URL, or "" without
// an accent.
//
// Returns the URL.
func (b *Brand) AccentURL() string {
	if b == nil || b.Accent == nil {
		return ""
	}
	return b.Accent.URL(AccentPath)
}

// Load builds the branding from a validated configuration, reading and
// checking the logo file.
//
// Parameters:
//   - cfg: the loaded configuration.
//
// Returns:
//   - *Brand: the branding; never nil.
//   - error: naming GONE_BRAND_LOGO when the logo can't be used.
func Load(cfg *config.Config) (*Brand, error) {
	b := &Brand{Name: cfg.BrandName, Links: links(cfg)}
	if cfg.BrandLogo != "" {
		logo, err := loadLogo(cfg.BrandLogo)
		if err != nil {
			return nil, fmt.Errorf("GONE_BRAND_LOGO: %w", err)
		}
		b.Logo = logo
	}
	if cfg.AccentLight != "" {
		b.Accent = newAsset(accentCSS(cfg.AccentLight, cfg.AccentDark), "text/css; charset=utf-8")
	}
	return b, nil
}

// links lists the configured footer links in display order.
//
// Parameters:
//   - cfg: the configuration.
//
// Returns the links; empty when none are set.
func links(cfg *config.Config) []Link {
	var out []Link
	for _, l := range []Link{{Kind: "contact", URL: cfg.ContactURL}, {Kind: "imprint", URL: cfg.ImprintURL}, {Kind: "privacy", URL: cfg.PrivacyURL}} {
		if l.URL != "" {
			l.Mailto = strings.HasPrefix(l.URL, "mailto:")
			out = append(out, l)
		}
	}
	return out
}

// loadLogo reads a logo file of at most MaxLogoBytes and identifies it as
// PNG or SVG.
//
// Parameters:
//   - path: the file path.
//
// Returns the asset, or an error that never quotes the file's contents.
func loadLogo(path string) (*Asset, error) {
	f, err := os.Open(path) // #nosec G304 -- the operator chooses this path
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err // the path is already in the message
		}
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxLogoBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(data) > MaxLogoBytes {
		return nil, fmt.Errorf("%s is larger than %d KiB", path, MaxLogoBytes>>10)
	}
	typ, err := logoType(data)
	if err != nil {
		return nil, fmt.Errorf("%s %w", path, err)
	}
	return newAsset(data, typ), nil
}

// logoType identifies a logo as PNG (by its signature) or SVG (an XML
// document whose root element is svg in the SVG namespace).
//
// Parameters:
//   - data: the file's bytes.
//
// Returns the Content-Type, or an error saying what is wrong.
func logoType(data []byte) (string, error) {
	if bytes.HasPrefix(data, pngMagic) {
		return "image/png", nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", errors.New("is neither a PNG nor an SVG file")
		}
		if el, ok := tok.(xml.StartElement); ok {
			if el.Name.Local != "svg" || el.Name.Space != svgNamespace {
				return "", fmt.Errorf("is not an SVG: its root element must be <svg xmlns=%q>", svgNamespace)
			}
			return "image/svg+xml", nil
		}
	}
}

// accentCSS returns the stylesheet that sets the accent tokens from the
// operator's light and dark colours. The soft and line tints are mixed from
// the accent, as the defaults in web/css/tokens.css are.
//
// Parameters:
//   - light, dark: #rrggbb colours, already validated.
//
// Returns the stylesheet.
func accentCSS(light, dark string) []byte {
	return []byte(fmt.Sprintf(`:root {
  --color-accent: light-dark(%[1]s, %[2]s);
  --color-accent-line: color-mix(in srgb, var(--color-accent) 40%%, transparent);
  --color-accent-soft: color-mix(in srgb, var(--color-accent) 10%%, transparent);
  --focus: light-dark(%[1]s, %[2]s);
  --focus-ring: color-mix(in srgb, var(--focus) 16%%, transparent);
}
`, light, dark))
}

// newAsset wraps data with its type and version.
//
// Parameters:
//   - data: the bytes.
//   - typ: the Content-Type.
//
// Returns the asset.
func newAsset(data []byte, typ string) *Asset {
	sum := sha256.Sum256(data)
	return &Asset{Data: data, Type: typ, Version: hex.EncodeToString(sum[:])[:12]}
}
