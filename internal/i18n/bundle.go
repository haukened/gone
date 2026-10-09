package i18n

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Bundle holds the loaded catalogs. It is read-only after Load and safe for
// concurrent use.
type Bundle struct {
	locales  []localeInfo                   // loaded locales in picker order
	catalogs map[string]map[string]*message // locale tag to key to message
	wire     map[string][]byte              // locale tag to its catalog as sent to the browser
	sums     map[string]string              // locale tag to a short hash of its wire catalog
}

// Options controls Load.
type Options struct {
	// Pseudo adds the PseudoLocale, generated from the base catalog. Use it
	// in development only.
	Pseudo bool
}

// Load reads every "<tag>.json" catalog in dir whose tag is a known locale
// and checks each against the base catalog: the same keys, and for every key
// the same argument and slot names. Other files (such as notes for
// translators) are ignored.
//
// Parameters:
//   - fsys: filesystem holding the catalogs.
//   - dir: directory inside fsys.
//   - opts: load options.
//
// Returns:
//   - *Bundle: the loaded catalogs.
//   - error: non-nil if the base catalog is missing or any catalog is invalid.
func Load(fsys fs.FS, dir string, opts Options) (*Bundle, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	b := &Bundle{catalogs: map[string]map[string]*message{}, wire: map[string][]byte{}, sums: map[string]string{}}
	for _, e := range entries {
		tag := strings.TrimSuffix(e.Name(), ".json")
		info, ok := lookupInfo(tag)
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || !ok || info.Tag != tag || tag == PseudoLocale {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		cat, err := parseCatalog(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		b.catalogs[tag] = cat
	}
	if err := b.finish(opts); err != nil {
		return nil, err
	}
	return b, nil
}

// finish checks the catalogs against the base, adds the pseudo-locale when
// asked, and prepares the browser copies.
func (b *Bundle) finish(opts Options) error {
	base, ok := b.catalogs[BaseLocale]
	if !ok {
		return fmt.Errorf("missing %s.json", BaseLocale)
	}
	for tag, cat := range b.catalogs {
		if err := checkCatalog(base, cat); err != nil {
			return fmt.Errorf("%s.json: %w", tag, err)
		}
	}
	if opts.Pseudo {
		b.catalogs[PseudoLocale] = pseudoCatalog(base)
	}
	for _, k := range known {
		cat, ok := b.catalogs[k.Tag]
		if !ok {
			continue
		}
		b.locales = append(b.locales, k)
		w, err := json.Marshal(wireCatalog(cat))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(w)
		b.wire[k.Tag], b.sums[k.Tag] = w, hex.EncodeToString(sum[:6])
	}
	return nil
}

// parseCatalog decodes a catalog file, skipping the "$schema" entry.
func parseCatalog(raw []byte) (map[string]*message, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	cat := make(map[string]*message, len(entries))
	for key, val := range entries {
		if key == "$schema" {
			continue
		}
		m, err := parseMessage(val)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		cat[key] = m
	}
	return cat, nil
}

// checkCatalog reports the first difference between cat and base: a missing
// or extra key, or a message whose argument or slot names differ.
func checkCatalog(base, cat map[string]*message) error {
	for _, key := range sortedKeys(base) {
		m, ok := cat[key]
		if !ok {
			return fmt.Errorf("missing key %s", key)
		}
		if got, want := m.signature(), base[key].signature(); got != want {
			return fmt.Errorf("%s: placeholders [%s], want [%s]", key, got, want)
		}
	}
	for key := range cat {
		if _, ok := base[key]; !ok {
			return fmt.Errorf("unknown key %s", key)
		}
	}
	return nil
}

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

// lookup returns the message for key in tag, falling back to the base
// locale; nil when the key is unknown.
func (b *Bundle) lookup(tag, key string) *message {
	if m, ok := b.catalogs[tag][key]; ok {
		return m
	}
	return b.catalogs[BaseLocale][key]
}

// T renders key in tag as plain text. Rich-text slot markers are dropped and
// their text kept. An unknown key renders as the key itself.
//
// Parameters:
//   - tag: the locale.
//   - key: the message key.
//   - args: the message's arguments.
//
// Returns:
//   - string: the text.
func (b *Bundle) T(tag, key string, args Args) string {
	return b.render(b.info(tag), key, args)
}

// render renders key as plain text with the given locale rules.
func (b *Bundle) render(info localeInfo, key string, args Args) string {
	m := b.lookup(info.Tag, key)
	if m == nil {
		return key
	}
	var sb strings.Builder
	for _, t := range m.choose(info, args) {
		switch t.kind {
		case tokText:
			sb.WriteString(t.text)
		case tokVar:
			sb.WriteString(b.formatValue(info, args[t.text]))
		}
	}
	return sb.String()
}

// choose returns the pattern for args: the plural variant for the input's
// category, falling back to "other".
func (m *message) choose(info localeInfo, args Args) pattern {
	if m.input == "" {
		return m.pattern
	}
	if p, ok := m.variants[pluralCategory(info, args[m.input])]; ok {
		return p
	}
	return m.variants["other"]
}

// slotTagRe matches a trusted opening tag supplied by a template for a slot.
var slotTagRe = regexp.MustCompile(`^<([a-z][a-z0-9]*)(\s[^<>]*)?>$`)

// Rich renders key in tag as HTML. All message text is escaped. Each
// {#name}…{/name} slot is wrapped in the opening tag the template supplied
// for name, marked data-i18n-slot so the browser can re-render the words
// around it, and closed by tag name. A slot with no tag renders as text.
//
// Parameters:
//   - tag: the locale.
//   - key: the message key.
//   - args: the message's arguments.
//   - slots: slot name to a trusted opening tag such as `<a href="/about">`.
//
// Returns:
//   - template.HTML: the markup.
func (b *Bundle) Rich(tag, key string, args Args, slots map[string]string) template.HTML {
	info := b.info(tag)
	m := b.lookup(info.Tag, key)
	if m == nil {
		return template.HTML(html.EscapeString(key)) // #nosec G203 -- escaped
	}
	var sb strings.Builder
	closing := ""
	for _, t := range m.choose(info, args) {
		switch t.kind {
		case tokText:
			sb.WriteString(html.EscapeString(t.text))
		case tokVar:
			sb.WriteString(html.EscapeString(b.formatValue(info, args[t.text])))
		case tokOpen:
			closing = writeSlotOpen(&sb, t.text, slots[t.text])
		case tokClose:
			sb.WriteString(closing)
			closing = ""
		}
	}
	return template.HTML(sb.String()) // #nosec G203 -- text escaped; tags are template literals
}

// writeSlotOpen writes a slot's opening tag with its data-i18n-slot marker
// and returns the matching closing tag, or writes nothing and returns "" when
// the tag is missing or malformed.
func writeSlotOpen(sb *strings.Builder, name, tag string) string {
	m := slotTagRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	sb.WriteString(tag[:len(tag)-1])
	sb.WriteString(` data-i18n-slot="` + name + `">`)
	return "</" + m[1] + ">"
}

// Format renders one argument value (a number, Bytes, Duration, …) in tag.
//
// Parameters:
//   - tag: the locale.
//   - v: the value.
//
// Returns:
//   - string: the formatted text.
func (b *Bundle) Format(tag string, v any) string {
	return b.formatValue(b.info(tag), v)
}

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

// String re-serializes a pattern in message syntax.
func (p pattern) String() string {
	var sb strings.Builder
	for _, t := range p {
		switch t.kind {
		case tokText:
			sb.WriteString(t.text)
		case tokVar:
			sb.WriteString("{" + t.text + "}")
		case tokOpen:
			sb.WriteString("{#" + t.text + "}")
		case tokClose:
			sb.WriteString("{/" + t.text + "}")
		}
	}
	return sb.String()
}
