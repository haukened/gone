package config

import (
	"fmt"
	"math"
	"strconv"
)

// Theme backgrounds an operator accent is drawn on (web/css/tokens.css:
// --pearl and --pane-solid for light, --dusk and --pane-solid for dark). A
// test keeps them in step with the stylesheet.
var (
	lightBackgrounds = []string{"#eeecf1", "#f8f7fa"}
	darkBackgrounds  = []string{"#121019", "#1c1926"}
)

// minAccentContrast is WCAG 2.2 AA for normal text, which the accent is
// used for (heading words, links).
const minAccentContrast = 4.5

// checkContrast reports an error when colour is below minAccentContrast on
// any of the backgrounds.
//
// Parameters:
//   - theme: "light" or "dark", for the message.
//   - colour: the accent, #rrggbb.
//   - backgrounds: the theme's background colours.
//
// Returns:
//   - error: naming GONE_BRAND_ACCENT with the lowest ratio, or nil.
func checkContrast(theme, colour string, backgrounds []string) error {
	lowest := math.Inf(1)
	for _, bg := range backgrounds {
		lowest = math.Min(lowest, contrastRatio(colour, bg))
	}
	if lowest < minAccentContrast {
		return fmt.Errorf("GONE_BRAND_ACCENT: the %s-theme colour %s has a contrast of %.2f:1 on that theme's background; it needs at least %.1f:1",
			theme, colour, lowest, minAccentContrast)
	}
	return nil
}

// isHexColor reports whether s is #rrggbb.
//
// Parameters:
//   - s: the candidate colour.
//
// Returns:
//   - bool: true for a # followed by six hex digits.
func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 32)
	return err == nil
}

// contrastRatio returns the WCAG 2.x contrast ratio of two #rrggbb colours.
//
// Parameters:
//   - a, b: the colours.
//
// Returns:
//   - float64: from 1 to 21.
func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// relativeLuminance returns the WCAG relative luminance of a #rrggbb colour.
//
// Parameters:
//   - hex: the colour; it must already be valid.
//
// Returns:
//   - float64: from 0 (black) to 1 (white).
func relativeLuminance(hex string) float64 {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	weights := [3]float64{0.2126, 0.7152, 0.0722}
	var l float64
	for i, w := range weights {
		c := float64((v>>(16-8*i))&0xff) / 255
		if c <= 0.04045 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		l += w * c
	}
	return l
}
