// Package i18n translates the web interface. It loads one message catalog
// per locale, picks a request's locale from its saved choice or
// Accept-Language, and renders messages with placeholders, plural variants,
// and rich-text slots. The browser re-renders the same catalogs when the
// visitor switches language, so the two sides share one message syntax.
package i18n

import (
	"sort"
	"strconv"
	"strings"
)

// BaseLocale is the source language every other catalog is checked against.
const BaseLocale = "en"

// PseudoLocale is the development-only pseudo-locale: English with accented
// letters and padding, to show untranslated and clipped text.
const PseudoLocale = "en-XA"

// CookieName is the cookie holding a visitor's chosen locale. The browser
// sets it when the visitor picks a language; the server only reads it.
const CookieName = "gone_lang"

// Locale describes one supported language.
type Locale struct {
	Tag  string `json:"tag"`  // BCP 47 tag, e.g. "pt-BR"
	Name string `json:"name"` // the language's own name, e.g. "Português (Brasil)"
	Dir  string `json:"dir"`  // text direction: "ltr" or "rtl"
}

// localeInfo holds the formatting rules of a known locale.
type localeInfo struct {
	Locale
	decimal  string               // decimal separator
	group    string               // thousands separator
	minGroup int                  // fewest integer digits before grouping applies, minus 3
	plural   func(n int64) string // CLDR cardinal category for an integer
}

// known lists every locale a catalog may be written for, in picker order.
var known = []localeInfo{
	{Locale: Locale{Tag: "en", Name: "English", Dir: "ltr"}, decimal: ".", group: ",", minGroup: 1, plural: pluralOneIfOne},
	{Locale: Locale{Tag: "es", Name: "Español", Dir: "ltr"}, decimal: ",", group: ".", minGroup: 2, plural: pluralRomance(false)},
	{Locale: Locale{Tag: "fr", Name: "Français", Dir: "ltr"}, decimal: ",", group: " ", minGroup: 1, plural: pluralRomance(true)},
	{Locale: Locale{Tag: "de", Name: "Deutsch", Dir: "ltr"}, decimal: ",", group: ".", minGroup: 1, plural: pluralOneIfOne},
	{Locale: Locale{Tag: "pt-BR", Name: "Português (Brasil)", Dir: "ltr"}, decimal: ",", group: ".", minGroup: 1, plural: pluralRomance(true)},
	{Locale: Locale{Tag: PseudoLocale, Name: "Pseudo (en-XA)", Dir: "ltr"}, decimal: ".", group: ",", minGroup: 1, plural: pluralOneIfOne},
}

// lookupInfo returns the rules for tag, matched case-insensitively.
func lookupInfo(tag string) (localeInfo, bool) {
	for _, k := range known {
		if strings.EqualFold(k.Tag, tag) {
			return k, true
		}
	}
	return localeInfo{}, false
}

// pluralOneIfOne is the CLDR cardinal rule for English and German integers.
func pluralOneIfOne(n int64) string {
	if n == 1 {
		return "one"
	}
	return "other"
}

// pluralRomance returns the CLDR cardinal rule for Spanish (zeroIsOne false)
// or French and Portuguese (zeroIsOne true) integers: "many" for nonzero
// multiples of a million.
func pluralRomance(zeroIsOne bool) func(n int64) string {
	return func(n int64) string {
		switch {
		case n == 1 || (zeroIsOne && n == 0):
			return "one"
		case n != 0 && n%1_000_000 == 0:
			return "many"
		default:
			return "other"
		}
	}
}

// acceptEntry is one language range from an Accept-Language header.
type acceptEntry struct {
	tag string
	q   float64
}

// parseAcceptLanguage returns the header's language ranges, highest quality
// first; ranges with q=0, wildcards, and malformed entries are dropped.
//
// Parameters:
//   - header: the Accept-Language value.
//
// Returns:
//   - []acceptEntry: the ranges in preference order, ties keeping header order.
func parseAcceptLanguage(header string) []acceptEntry {
	var out []acceptEntry
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.TrimSpace(fields[0])
		if tag == "" || tag == "*" || len(tag) > 35 {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			name, val, ok := strings.Cut(strings.TrimSpace(f), "=")
			if ok && strings.EqualFold(strings.TrimSpace(name), "q") {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
				if err != nil {
					parsed = 0
				}
				q = parsed
			}
		}
		if q > 0 {
			out = append(out, acceptEntry{tag: tag, q: q})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].q > out[j].q })
	return out
}

// baseLanguage returns the language subtag of tag, lowercased.
func baseLanguage(tag string) string {
	base, _, _ := strings.Cut(tag, "-")
	base, _, _ = strings.Cut(base, "_")
	return strings.ToLower(base)
}
