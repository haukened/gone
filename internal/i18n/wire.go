package i18n

import (
	"strings"
)

// Messages returns the messages under any of prefixes for tag, in the form
// the browser reads (see wireCatalog), for inlining into a page.
//
// Parameters:
//   - tag: the locale.
//   - prefixes: key prefixes such as "js." and "common.".
//
// Returns:
//   - map[string]any: key to wire message.
func (b *Bundle) Messages(tag string, prefixes ...string) map[string]any {
	cat, ok := b.catalogs[tag]
	if !ok {
		cat = b.catalogs[BaseLocale]
	}
	out := map[string]any{}
	for key, m := range cat {
		for _, p := range prefixes {
			if strings.HasPrefix(key, p) {
				out[key] = m.wire()
				break
			}
		}
	}
	return out
}

// Catalog returns tag's full catalog as JSON for the browser and a short
// hash of it for cache-busting URLs.
//
// Returns:
//   - []byte: the catalog, or nil when tag is not loaded.
//   - string: the hash.
func (b *Bundle) Catalog(tag string) ([]byte, string) {
	return b.wire[tag], b.sums[tag]
}

// wireCatalog converts a catalog to the browser form.
func wireCatalog(cat map[string]*message) map[string]any {
	out := make(map[string]any, len(cat))
	for key, m := range cat {
		out[key] = m.wire()
	}
	return out
}

// wire returns a message as the browser reads it: a pattern string, or for
// plural variants an object {"$plural": input, "<category>": pattern, …}.
func (m *message) wire() any {
	if m.input == "" {
		return m.pattern.String()
	}
	out := map[string]string{"$plural": m.input}
	for cat, p := range m.variants {
		out[cat] = p.String()
	}
	return out
}
