package i18n

import (
	"sort"
	"strings"
)

// sortedKeys returns m's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Locales returns the loaded locales in picker order.
func (b *Bundle) Locales() []Locale {
	out := make([]Locale, len(b.locales))
	for i, l := range b.locales {
		out[i] = l.Locale
	}
	return out
}

// Has reports whether tag is a loaded locale.
func (b *Bundle) Has(tag string) bool {
	_, ok := b.catalogs[tag]
	return ok
}

// Keys returns the base catalog's keys, sorted.
func (b *Bundle) Keys() []string {
	return sortedKeys(b.catalogs[BaseLocale])
}

// Locale returns the metadata for a loaded tag, or the base locale's.
func (b *Bundle) Locale(tag string) Locale {
	return b.info(tag).Locale
}

// info returns the formatting rules for a loaded tag, or the base locale's.
func (b *Bundle) info(tag string) localeInfo {
	for _, l := range b.locales {
		if l.Tag == tag {
			return l
		}
	}
	info, _ := lookupInfo(BaseLocale)
	return info
}

// Match picks the locale for a request: the saved choice when it names a
// loaded locale, else the first Accept-Language range that matches a loaded
// locale exactly or by language ("es-MX" to "es", "pt" to "pt-BR"), else
// the base locale. The pseudo-locale is only chosen explicitly.
//
// Parameters:
//   - saved: the gone_lang cookie value, or "".
//   - acceptLanguage: the Accept-Language header, or "".
//
// Returns:
//   - string: a loaded locale tag.
func (b *Bundle) Match(saved, acceptLanguage string) string {
	if b.Has(saved) {
		return saved
	}
	for _, entry := range parseAcceptLanguage(acceptLanguage) {
		if tag := b.matchOne(entry.tag); tag != "" {
			return tag
		}
	}
	return BaseLocale
}

// matchOne maps one requested tag to a loaded locale, or "".
func (b *Bundle) matchOne(requested string) string {
	for _, l := range b.locales {
		if l.Tag != PseudoLocale && strings.EqualFold(l.Tag, requested) {
			return l.Tag
		}
	}
	base := baseLanguage(requested)
	for _, l := range b.locales {
		if l.Tag != PseudoLocale && baseLanguage(l.Tag) == base {
			return l.Tag
		}
	}
	return ""
}
