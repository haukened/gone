package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// tokenKind classifies one piece of a parsed message.
type tokenKind int

const (
	tokText  tokenKind = iota // literal text
	tokVar                    // {name}: an argument
	tokOpen                   // {#name}: start of a rich-text slot
	tokClose                  // {/name}: end of a rich-text slot
)

// token is one piece of a parsed message pattern.
type token struct {
	kind tokenKind
	text string // the literal text, or the variable or slot name
}

// pattern is a parsed message string.
type pattern []token

// message is one catalog entry: a single pattern, or plural variants chosen
// by the CLDR category of one numeric input.
type message struct {
	pattern  pattern
	input    string             // plural input variable; empty for a plain message
	variants map[string]pattern // plural category ("one", "many", "other", ...) to pattern
}

// nameRe matches a variable or slot name.
var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// pluralCategories are the CLDR cardinal categories a variant may name.
var pluralCategories = map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}

// parsePattern splits a message string into tokens. Braces are only allowed
// around a name: {name}, {#name}, or {/name}. Slots must be balanced and may
// not nest.
//
// Parameters:
//   - s: the message text.
//
// Returns:
//   - pattern: the tokens.
//   - error: non-nil for a stray brace, a bad name, or unbalanced slots.
func parsePattern(s string) (pattern, error) {
	var out pattern
	open := ""
	for len(s) > 0 {
		toks, rest, err := nextTokens(s)
		if err != nil {
			return nil, err
		}
		for _, tok := range toks {
			if open, err = trackSlot(open, tok); err != nil {
				return nil, err
			}
		}
		out = append(out, toks...)
		s = rest
	}
	if open != "" {
		return nil, fmt.Errorf("slot %q is not closed", open)
	}
	return out, nil
}

// nextTokens reads the text up to the next placeholder and the placeholder
// itself from the start of s.
//
// Returns:
//   - []token: one or two tokens.
//   - string: the rest of s.
//   - error: non-nil for a stray or unclosed brace or a bad name.
func nextTokens(s string) ([]token, string, error) {
	i := strings.IndexAny(s, "{}")
	if i < 0 {
		return []token{{kind: tokText, text: s}}, "", nil
	}
	if s[i] == '}' {
		return nil, "", errors.New("stray }")
	}
	var toks []token
	if i > 0 {
		toks = append(toks, token{kind: tokText, text: s[:i]})
	}
	end := strings.IndexByte(s[i:], '}')
	if end < 0 {
		return nil, "", errors.New("unclosed {")
	}
	tok, err := parsePlaceholder(s[i+1 : i+end])
	if err != nil {
		return nil, "", err
	}
	return append(toks, tok), s[i+end+1:], nil
}

// parsePlaceholder parses the text between braces.
func parsePlaceholder(inner string) (token, error) {
	kind := tokVar
	switch {
	case strings.HasPrefix(inner, "#"):
		kind, inner = tokOpen, inner[1:]
	case strings.HasPrefix(inner, "/"):
		kind, inner = tokClose, inner[1:]
	}
	if !nameRe.MatchString(inner) {
		return token{}, fmt.Errorf("bad placeholder name %q", inner)
	}
	return token{kind: kind, text: inner}, nil
}

// trackSlot updates the open slot name for tok, rejecting nesting and
// mismatched closes.
func trackSlot(open string, tok token) (string, error) {
	switch tok.kind {
	case tokOpen:
		if open != "" {
			return "", fmt.Errorf("slot %q nested in %q", tok.text, open)
		}
		return tok.text, nil
	case tokClose:
		if open != tok.text {
			return "", fmt.Errorf("close of slot %q does not match", tok.text)
		}
		return "", nil
	}
	return open, nil
}

// variantMessage is the inlang message-format shape for plural variants:
//
//	[{"declarations": ["input count", "local countPlural = count: plural"],
//	  "selectors": ["countPlural"],
//	  "match": {"countPlural=one": "…", "countPlural=*": "…"}}]
type variantMessage struct {
	Declarations []string          `json:"declarations"`
	Selectors    []string          `json:"selectors"`
	Match        map[string]string `json:"match"`
}

// localDeclRe matches "local <selector> = <input>: plural".
var localDeclRe = regexp.MustCompile(`^local\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([A-Za-z_][A-Za-z0-9_]*)\s*:\s*plural$`)

// parseMessage decodes one catalog value: a string, or a one-element array
// holding plural variants.
//
// Parameters:
//   - raw: the JSON value.
//
// Returns:
//   - *message: the parsed message.
//   - error: non-nil for any other shape or a bad pattern.
func parseMessage(raw json.RawMessage) (*message, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		p, err := parsePattern(s)
		if err != nil {
			return nil, err
		}
		return &message{pattern: p}, nil
	}
	var vs []variantMessage
	if err := json.Unmarshal(raw, &vs); err != nil || len(vs) != 1 {
		return nil, errors.New("must be a string or one plural variant object")
	}
	return parseVariants(vs[0])
}

// parseVariants validates a plural variant object and parses its patterns.
func parseVariants(v variantMessage) (*message, error) {
	if len(v.Selectors) != 1 {
		return nil, errors.New("variants need exactly one selector")
	}
	input := pluralInput(v)
	if input == "" {
		return nil, fmt.Errorf("selector %q is not declared as a plural", v.Selectors[0])
	}
	msg := &message{input: input, variants: map[string]pattern{}}
	for key, text := range v.Match {
		cat, err := matchCategory(key, v.Selectors[0])
		if err != nil {
			return nil, err
		}
		p, err := parsePattern(text)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		msg.variants[cat] = p
	}
	if _, ok := msg.variants["other"]; !ok {
		return nil, errors.New("variants need an \"other\" (*) case")
	}
	return msg, nil
}

// pluralInput returns the input variable a variant object's selector is
// declared over ("local <selector> = <input>: plural"), or "".
func pluralInput(v variantMessage) string {
	for _, d := range v.Declarations {
		if m := localDeclRe.FindStringSubmatch(strings.TrimSpace(d)); m != nil && m[1] == v.Selectors[0] {
			return m[2]
		}
	}
	return ""
}

// matchCategory returns the plural category a match key names, with "*"
// meaning "other".
//
// Parameters:
//   - key: the match key, "<selector>=<category>".
//   - selector: the variant object's selector.
//
// Returns:
//   - string: the category.
//   - error: non-nil for another selector or an unknown category.
func matchCategory(key, selector string) (string, error) {
	sel, cat, ok := strings.Cut(key, "=")
	if !ok || sel != selector {
		return "", fmt.Errorf("bad match key %q", key)
	}
	if cat == "*" {
		cat = "other"
	}
	if !pluralCategories[cat] {
		return "", fmt.Errorf("unknown plural category %q", cat)
	}
	return cat, nil
}

// signature lists a message's argument and slot names, sorted, so catalogs
// can be checked against the base locale.
func (m *message) signature() string {
	seen := map[string]bool{}
	if m.input != "" {
		seen["$"+m.input] = true
	}
	add := func(p pattern) {
		for _, t := range p {
			switch t.kind {
			case tokVar:
				seen["$"+t.text] = true
			case tokOpen:
				seen["#"+t.text] = true
			}
		}
	}
	add(m.pattern)
	for _, p := range m.variants {
		add(p)
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
