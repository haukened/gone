package main

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/i18n"
)

// i18nPagePaths are every page, rendered in each locale.
var i18nPagePaths = []string{"/", "/about", "/secret/x", "/manage/x", "/request", "/request/x", "/reply/x", "/no-such-page"}

// getIn renders path with a saved locale and an Accept-Language header.
//
// Parameters:
//   - h: the router.
//   - path: the page.
//   - saved: the gone_lang cookie value, or "".
//   - accept: the Accept-Language header, or "".
//
// Returns:
//   - *httptest.ResponseRecorder: the response.
func getIn(h http.Handler, path, saved, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if saved != "" {
		req.AddCookie(&http.Cookie{Name: i18n.CookieName, Value: saved})
	}
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// keyedText matches an element whose whole content is the text of its
// data-i18n message (no child elements, args or values).
var keyedText = regexp.MustCompile(`<[a-z0-9]+ [^>]*data-i18n="([a-z][\w.]*)"([^>]*)>([^<]*)</`)

// TestPages_EveryLocale renders every page in every locale and checks the
// document language, the headers, and that each simple keyed element shows
// its catalog text.
func TestPages_EveryLocale(t *testing.T) {
	h := pageRouter(t)
	b := testBundle(t)
	for _, loc := range b.Locales() {
		for _, path := range i18nPagePaths {
			t.Run(loc.Tag+path, func(t *testing.T) {
				rr := getIn(h, path, loc.Tag, "")
				body := rr.Body.String()
				if want := `<html lang="` + loc.Tag + `" dir="` + loc.Dir + `">`; !strings.Contains(body, want) {
					t.Errorf("missing %s", want)
				}
				if got := rr.Header().Get("Content-Language"); got != loc.Tag {
					t.Errorf("Content-Language = %q", got)
				}
				if !strings.Contains(strings.Join(rr.Header().Values("Vary"), ","), "Accept-Language") {
					t.Errorf("Vary = %v", rr.Header().Values("Vary"))
				}
				checkKeyedText(t, b, loc.Tag, body)
			})
		}
	}
}

// checkKeyedText compares each simple keyed element's text with the catalog.
func checkKeyedText(t *testing.T, b *i18n.Bundle, tag, body string) {
	t.Helper()
	for _, m := range keyedText.FindAllStringSubmatch(body, -1) {
		key, attrs, text := m[1], m[2], html.UnescapeString(m[3])
		if strings.Contains(attrs, "data-i18n-args") || strings.Contains(string(b.Rich(tag, key, nil, nil)), "<") {
			continue
		}
		if want := b.T(tag, key, nil); text != want {
			t.Errorf("%s: %q, want %q", key, text, want)
		}
	}
}

// TestPages_LocaleDetection checks the saved choice beats Accept-Language,
// which beats the default.
func TestPages_LocaleDetection(t *testing.T) {
	h := pageRouter(t)
	b := testBundle(t)
	other := ""
	for _, l := range b.Locales() {
		if l.Tag != i18n.BaseLocale && l.Tag != i18n.PseudoLocale {
			other = l.Tag
		}
	}
	cases := []struct{ saved, accept, want string }{
		{"", "", "en"},
		{"", "xx-YY, zz;q=0.5", "en"},
		{"nope", "", "en"},
		{i18n.PseudoLocale, "en", i18n.PseudoLocale},
	}
	if other != "" {
		cases = append(cases,
			struct{ saved, accept, want string }{"", other + ";q=0.9, en;q=0.1", other},
			struct{ saved, accept, want string }{"en", other, "en"},
		)
	}
	for _, tc := range cases {
		if got := getIn(h, "/", tc.saved, tc.accept).Header().Get("Content-Language"); got != tc.want {
			t.Errorf("saved %q accept %q: %q, want %q", tc.saved, tc.accept, got, tc.want)
		}
	}
}

var (
	// stripped removes markup whose text is not translated: scripts (the
	// inline catalog), icons, the picker (each language in its own name),
	// and the decorative vault.
	stripped  = regexp.MustCompile(`(?s)<script.*?</script>|<svg.*?</svg>|<select.*?</select>|<pre class="vault".*?</pre>|<title.*?</title>`)
	tagRe     = regexp.MustCompile(`<[^>]*>`)
	asciiWord = regexp.MustCompile(`[A-Za-z]{2,}`)
)

// untranslatedAllowed are the visible words that stay as they are in every
// language: the brand (and the wordmark's "one" after its G mark), the
// version, and the QR and language-code labels.
var untranslatedAllowed = map[string]bool{"Gone": true, "one": true, "test": true, "QR": true, "EN": true}

// TestPages_PseudoLocaleCoversAllText renders every page in the
// pseudo-locale, where every translated letter is accented: any plain ASCII
// word left on the page is text that skipped translation.
func TestPages_PseudoLocaleCoversAllText(t *testing.T) {
	h := pageRouter(t)
	for _, path := range i18nPagePaths {
		body := getIn(h, path, i18n.PseudoLocale, "").Body.String()
		text := html.UnescapeString(tagRe.ReplaceAllString(stripped.ReplaceAllString(body, " "), " "))
		for _, w := range asciiWord.FindAllString(text, -1) {
			if !untranslatedAllowed[w] {
				t.Errorf("%s: untranslated %q", path, w)
			}
		}
		for _, attr := range regexp.MustCompile(`\s(?:aria-label|placeholder|title)="([^"]*)"`).FindAllStringSubmatch(body, -1) {
			for _, w := range asciiWord.FindAllString(html.UnescapeString(attr[1]), -1) {
				if !untranslatedAllowed[w] {
					t.Errorf("%s: untranslated attribute text %q", path, w)
				}
			}
		}
	}
}

// keyRefs find message keys in source files.
var (
	tmplKeyRe  = regexp.MustCompile(`(?:\bt|\bta|\brich)\s+"([a-z][\w.]*)"|data-i18n="([a-z][\w.]*)"`)
	attrKeysRe = regexp.MustCompile(`data-i18n-attr="([^"{]*)"`)
	jsKeyRe    = regexp.MustCompile(`'((?:js|common|send|secret|manage|request|detail|reply|result|compose|error)\.[\w.]+)'`)
	goKeyRe    = regexp.MustCompile(`"((?:common|error)\.[\w.]+)"`)
)

// dynamicKeyPrefixes are message keys composed at run time, which the source
// scan cannot see.
var dynamicKeyPrefixes = []string{"common.size.", "common.duration.", "js.pass.", "error.notFound."}

// TestMessages_KeysUsed checks that every key the templates, scripts, and
// handlers use exists in the base catalog, and that the catalog has no keys
// nothing uses.
func TestMessages_KeysUsed(t *testing.T) {
	used := map[string]bool{}
	scan(t, "../../web/*.tmpl.html", func(s string) {
		for _, m := range tmplKeyRe.FindAllStringSubmatch(s, -1) {
			used[m[1]+m[2]] = true
		}
		for _, m := range attrKeysRe.FindAllStringSubmatch(s, -1) {
			for _, pair := range strings.Split(m[1], ";") {
				if _, key, ok := strings.Cut(pair, ":"); ok {
					used[key] = true
				}
			}
		}
	})
	scan(t, "../../web/js/*.js", func(s string) { collect(used, jsKeyRe, s) })
	scan(t, "../../internal/httpx/*.go", func(s string) { collect(used, goKeyRe, s) })
	b := testBundle(t)
	keys := map[string]bool{}
	for _, k := range b.Keys() {
		keys[k] = true
	}
	var missing, unused []string
	for k := range used {
		if !keys[k] && k != "error.notFound" && !strings.HasSuffix(k, ".") {
			missing = append(missing, k)
		}
	}
	for k := range keys {
		if !used[k] && !hasAnyPrefix(k, dynamicKeyPrefixes) {
			unused = append(unused, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(unused)
	if len(missing) > 0 || len(unused) > 0 {
		t.Fatalf("missing from en.json: %v\nunused in en.json: %v", missing, unused)
	}
}

// scan calls fn with the contents of each non-test file matching pattern.
func scan(t *testing.T, pattern string, fn func(string)) {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		t.Fatalf("glob %s: %v (%d files)", pattern, err, len(files))
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f) // #nosec G304 -- repository files named by a fixed glob
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		fn(string(data))
	}
}

// collect adds each first submatch of re in s to used.
func collect(used map[string]bool, re *regexp.Regexp, s string) {
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		used[m[1]] = true
	}
}

// hasAnyPrefix reports whether s starts with any of prefixes.
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
