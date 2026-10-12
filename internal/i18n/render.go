package i18n

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

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
		return template.HTML(html.EscapeString(key)) // #nosec G203 -- escaped // nosemgrep
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
	return template.HTML(sb.String()) // #nosec G203 -- text escaped; tags are template literals // nosemgrep
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
