package i18n

import "strings"

// pseudoAccents maps ASCII letters to accented look-alikes, so text that
// skipped translation stands out as plain ASCII.
var pseudoAccents = strings.NewReplacer(
	"a", "á", "b", "ƀ", "c", "ç", "d", "ð", "e", "é", "f", "ƒ", "g", "ĝ", "h", "ĥ", "i", "í", "j", "ĵ",
	"k", "ķ", "l", "ĺ", "m", "ɱ", "n", "ñ", "o", "ó", "p", "þ", "q", "ǫ", "r", "ŕ", "s", "š", "t", "ţ",
	"u", "ú", "v", "ṽ", "w", "ŵ", "x", "ẋ", "y", "ý", "z", "ž",
	"A", "Á", "B", "Ɓ", "C", "Ç", "D", "Ð", "E", "É", "F", "Ƒ", "G", "Ĝ", "H", "Ĥ", "I", "Í", "J", "Ĵ",
	"K", "Ķ", "L", "Ĺ", "M", "Ṁ", "N", "Ñ", "O", "Ó", "P", "Þ", "Q", "Ǫ", "R", "Ŕ", "S", "Š", "T", "Ţ",
	"U", "Ú", "V", "Ṽ", "W", "Ŵ", "X", "Ẋ", "Y", "Ý", "Z", "Ž",
)

// pseudoCatalog derives the pseudo-locale from the base catalog: letters
// accented, each message bracketed, and padded by about a third so layouts
// that clip longer languages show it. Placeholders and slots are kept.
func pseudoCatalog(base map[string]*message) map[string]*message {
	out := make(map[string]*message, len(base))
	for key, m := range base {
		pm := &message{input: m.input}
		if m.input == "" {
			pm.pattern = pseudoPattern(m.pattern)
		} else {
			pm.variants = make(map[string]pattern, len(m.variants))
			for cat, p := range m.variants {
				pm.variants[cat] = pseudoPattern(p)
			}
		}
		out[key] = pm
	}
	return out
}

// pseudoPattern accents and pads one pattern.
func pseudoPattern(p pattern) pattern {
	letters := 0
	out := pattern{{kind: tokText, text: "⟦"}}
	for _, t := range p {
		if t.kind == tokText {
			letters += len(t.text)
			t = token{kind: tokText, text: pseudoAccents.Replace(t.text)}
		}
		out = append(out, t)
	}
	// Padding in short groups can wrap, as real longer text would.
	pad := strings.TrimSpace(strings.Repeat("··· ", (letters+8)/9))
	return append(out, token{kind: tokText, text: " " + pad + "⟧"})
}
