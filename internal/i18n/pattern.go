package i18n

import (
	"errors"
	"fmt"
	"regexp"
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

// nameRe matches a variable or slot name.
var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
