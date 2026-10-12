package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// message is one catalog entry: a single pattern, or plural variants chosen
// by the CLDR category of one numeric input.
type message struct {
	pattern  pattern
	input    string             // plural input variable; empty for a plain message
	variants map[string]pattern // plural category ("one", "many", "other", ...) to pattern
}

// pluralCategories are the CLDR cardinal categories a variant may name.
var pluralCategories = map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}

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
