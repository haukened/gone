package i18n

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// testEN is a small base catalog covering every message feature.
const testEN = `{
	"$schema": "https://inlang.com/schema/inlang-message-format",
	"common.size.b": "{n} B",
	"common.size.kb": "{n} KB",
	"common.size.mb": "{n} MB",
	"common.size.gb": "{n} GB",
	"common.size.tb": "{n} TB",
	"common.size.pb": "{n} PB",
	"common.duration.short.days": "{n}d",
	"common.duration.short.hours": "{n}h",
	"common.duration.short.minutes": "{n}m",
	"common.duration.short.seconds": "{n}s",
	"common.duration.long.days": [{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=one": "1 day", "nPlural=*": "{n} days"}}],
	"common.duration.long.hours": [{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=one": "1 hour", "nPlural=*": "{n} hours"}}],
	"common.duration.long.minutes": "{n} min",
	"common.duration.long.seconds": "{n} sec",
	"js.greet": "Hello {name}",
	"page.files": [{"declarations": ["input count", "local countPlural = count: plural"], "selectors": ["countPlural"], "match": {"countPlural=one": "a file", "countPlural=many": "{count} files (many)", "countPlural=*": "{count} files"}}],
	"page.rich": "Read the {#link}protocol{/link} & <notes>.",
	"page.range": "From {min} to {max}."
}`

// testES translates testEN.
const testES = `{
	"common.size.b": "{n} B",
	"common.size.kb": "{n} KB",
	"common.size.mb": "{n} MB",
	"common.size.gb": "{n} GB",
	"common.size.tb": "{n} TB",
	"common.size.pb": "{n} PB",
	"common.duration.short.days": "{n} d",
	"common.duration.short.hours": "{n} h",
	"common.duration.short.minutes": "{n} min",
	"common.duration.short.seconds": "{n} s",
	"common.duration.long.days": [{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=one": "1 día", "nPlural=*": "{n} días"}}],
	"common.duration.long.hours": [{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=one": "1 hora", "nPlural=*": "{n} horas"}}],
	"common.duration.long.minutes": "{n} min",
	"common.duration.long.seconds": "{n} s",
	"js.greet": "Hola {name}",
	"page.files": [{"declarations": ["input count", "local countPlural = count: plural"], "selectors": ["countPlural"], "match": {"countPlural=one": "un archivo", "countPlural=many": "{count} de archivos", "countPlural=*": "{count} archivos"}}],
	"page.rich": "Lee el {#link}protocolo{/link} y <notas>.",
	"page.range": "De {min} a {max}."
}`

// loadTest loads the test catalogs plus any extra files.
func loadTest(t *testing.T, opts Options, extra map[string]string) *Bundle {
	t.Helper()
	b, err := Load(testFS(extra), "messages", opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return b
}

// testFS returns the test catalogs, overridden or extended by extra.
func testFS(extra map[string]string) fstest.MapFS {
	files := map[string]string{"en.json": testEN, "es.json": testES, "_context.json": `{"note": "ignored"}`, "xx.json": `not json`}
	for k, v := range extra {
		files[k] = v
	}
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys["messages/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestLoadLocales(t *testing.T) {
	b := loadTest(t, Options{}, nil)
	got := b.Locales()
	if len(got) != 2 || got[0].Tag != "en" || got[1] != (Locale{Tag: "es", Name: "Español", Dir: "ltr"}) {
		t.Fatalf("Locales = %+v", got)
	}
	if !b.Has("es") || b.Has("fr") || b.Has(PseudoLocale) {
		t.Fatal("Has mismatch")
	}
	if b.Locale("fr").Tag != "en" || b.Locale("es").Name != "Español" {
		t.Fatal("Locale fallback mismatch")
	}
	if keys := b.Keys(); len(keys) != 18 || keys[0] != "common.duration.long.days" {
		t.Fatalf("Keys = %v", keys)
	}
	p := loadTest(t, Options{Pseudo: true}, nil)
	if !p.Has(PseudoLocale) || p.Locales()[2].Tag != PseudoLocale {
		t.Fatalf("pseudo locales = %+v", p.Locales())
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"no base", map[string]string{"en.json": ""}, "missing en.json"},
		{"bad json", map[string]string{"es.json": `{`}, "es.json"},
		{"missing key", map[string]string{"es.json": `{"js.greet": "Hola {name}"}`}, "missing key"},
		{"extra key", map[string]string{"es.json": strings.Replace(testES, `"page.range"`, `"page.extra": "x", "page.range"`, 1)}, "unknown key page.extra"},
		{"placeholder", map[string]string{"es.json": strings.Replace(testES, "Hola {name}", "Hola {nombre}", 1)}, "js.greet: placeholders"},
		{"slot", map[string]string{"es.json": strings.Replace(testES, "{#link}protocolo{/link}", "protocolo", 1)}, "page.rich"},
		{"bad message", map[string]string{"es.json": strings.Replace(testES, `"Hola {name}"`, `42`, 1)}, "js.greet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := testFS(tc.files)
			if tc.files["en.json"] == "" && tc.name == "no base" {
				delete(fsys, "messages/en.json")
			}
			_, err := Load(fsys, "messages", Options{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := Load(fstest.MapFS{}, "missing", Options{}); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestParsePatternErrors(t *testing.T) {
	for _, s := range []string{"a } b", "a {b", "{bad name}", "{#a}x{#b}y{/b}{/a}", "{#a}x{/b}", "{#a}x", "{1x}"} {
		if _, err := parsePattern(s); err == nil {
			t.Errorf("parsePattern(%q) succeeded", s)
		}
	}
	p, err := parsePattern("a {b} {#c}d{/c}")
	if err != nil || p.String() != "a {b} {#c}d{/c}" {
		t.Fatalf("round trip = %q, %v", p.String(), err)
	}
}

func TestParseVariantErrors(t *testing.T) {
	cases := []string{
		`[{"declarations": ["input n"], "selectors": ["nPlural"], "match": {"nPlural=*": "x"}}]`,
		`[{"declarations": ["input n", "local nPlural = n: plural"], "selectors": [], "match": {}}]`,
		`[{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=one": "x"}}]`,
		`[{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"other=*": "x"}}]`,
		`[{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=lots": "x", "nPlural=*": "y"}}]`,
		`[{"declarations": ["input n", "local nPlural = n: plural"], "selectors": ["nPlural"], "match": {"nPlural=*": "{"}}]`,
		`[]`,
	}
	for _, c := range cases {
		if _, err := parseMessage(json.RawMessage(c)); err == nil {
			t.Errorf("parseMessage(%s) succeeded", c)
		}
	}
}

func TestMatch(t *testing.T) {
	b := loadTest(t, Options{Pseudo: true}, map[string]string{"pt-BR.json": testES})
	cases := []struct{ saved, accept, want string }{
		{"", "", "en"},
		{"es", "en", "es"},
		{"zz", "es-MX,es;q=0.9", "es"},
		{"", "fr-CA, de;q=0.5", "en"},
		{"", "de;q=0.5, es-AR;q=0.8", "es"},
		{"", "pt", "pt-BR"},
		{"", "pt-PT,en;q=0.1", "pt-BR"},
		{"", "PT-br", "pt-BR"},
		{"", "es;q=0, en", "en"},
		{"", "*, es;q=bad", "en"},
		{"", "en-XA", "en"},
		{PseudoLocale, "", PseudoLocale},
		{"", strings.Repeat("x", 40) + ", es", "es"},
	}
	for _, tc := range cases {
		if got := b.Match(tc.saved, tc.accept); got != tc.want {
			t.Errorf("Match(%q, %q) = %q, want %q", tc.saved, tc.accept, got, tc.want)
		}
	}
}

func TestTRendering(t *testing.T) {
	b := loadTest(t, Options{}, nil)
	cases := []struct {
		tag, key string
		args     Args
		want     string
	}{
		{"en", "js.greet", Args{"name": "Ana"}, "Hello Ana"},
		{"es", "js.greet", Args{"name": "Ana"}, "Hola Ana"},
		{"fr", "js.greet", Args{"name": "Ana"}, "Hello Ana"},
		{"en", "page.files", Args{"count": 1}, "a file"},
		{"en", "page.files", Args{"count": int64(3)}, "3 files"},
		{"en", "page.files", Args{"count": 2_000_000}, "2,000,000 files"},
		{"es", "page.files", Args{"count": 2_000_000}, "2.000.000 de archivos"},
		{"es", "page.files", Args{"count": Number{Value: 1, Digits: 0}}, "un archivo"},
		{"es", "page.files", Args{"count": Number{Value: 1.5, Digits: 1}}, "1,5 archivos"},
		{"en", "page.files", Args{"count": "x"}, "x files"},
		{"en", "page.rich", nil, "Read the protocol & <notes>."},
		{"en", "page.unknown", nil, "page.unknown"},
		{"en", "page.range", Args{"min": Duration{Seconds: 300, Style: "short"}, "max": Duration{Seconds: 86400 * 2}}, "From 5m to 2 days."},
		{"es", "page.range", Args{"min": Duration{Seconds: 3600, Style: "short"}, "max": Duration{Seconds: 3600}}, "De 1 h a 1 hora."},
		{"en", "page.range", Args{"min": Duration{Seconds: 45, Style: "short"}, "max": Duration{Seconds: 0}}, "From 45s to 0 sec."},
		{"en", "page.range", Args{"min": Bytes(512), "max": Bytes(10 << 20)}, "From 512 B to 10.0 MB."},
		{"es", "page.range", Args{"min": Bytes(1536), "max": Bytes(1 << 60)}, "De 1,5 KB a 1024,0 PB."},
		{"en", "page.range", Args{"min": 1.25, "max": -1234567}, "From 1.25 to -1,234,567."},
		{"en", "page.range", Args{"min": Number{Value: -2.5, Digits: 1}, "max": struct{}{}}, "From -2.5 to ."},
	}
	for _, tc := range cases {
		if got := b.T(tc.tag, tc.key, tc.args); got != tc.want {
			t.Errorf("T(%s, %s) = %q, want %q", tc.tag, tc.key, got, tc.want)
		}
	}
}

func TestFormatLocales(t *testing.T) {
	cases := []struct {
		tag  string
		v    float64
		want string
	}{
		{"en", 1234.5, "1,234.5"},
		{"es", 1234.5, "1234,5"},
		{"es", 12345.5, "12.345,5"},
		{"fr", 1234.5, "1 234,5"},
		{"de", 1234.5, "1.234,5"},
		{"pt-BR", 1234.5, "1.234,5"},
	}
	for _, tc := range cases {
		info, _ := lookupInfo(tc.tag)
		if got := formatDecimal(info, tc.v, 1); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.tag, got, tc.want)
		}
	}
	info, _ := lookupInfo("en")
	if got := formatDecimal(info, 0, -1); got != "0" {
		t.Errorf("zero = %q", got)
	}
	if got := formatValueNaN(); got != "NaN" {
		t.Errorf("NaN = %q", got)
	}
}

// formatValueNaN formats NaN, which bypasses grouping.
func formatValueNaN() string {
	info, _ := lookupInfo("en")
	var nan float64
	return formatDecimal(info, nan/nan, 1)
}

func TestPluralRules(t *testing.T) {
	cases := []struct {
		tag  string
		n    int64
		want string
	}{
		{"en", 1, "one"}, {"en", 0, "other"}, {"de", 1, "one"}, {"de", 2, "other"},
		{"es", 1, "one"}, {"es", 0, "other"}, {"es", 1_000_000, "many"},
		{"fr", 0, "one"}, {"fr", 1, "one"}, {"fr", 2, "other"}, {"fr", 3_000_000, "many"},
		{"pt-BR", 0, "one"}, {"pt-BR", 5, "other"},
	}
	for _, tc := range cases {
		info, _ := lookupInfo(tc.tag)
		if got := info.plural(tc.n); got != tc.want {
			t.Errorf("%s plural(%d) = %s, want %s", tc.tag, tc.n, got, tc.want)
		}
	}
}

func TestRich(t *testing.T) {
	b := loadTest(t, Options{}, nil)
	slots := map[string]string{"link": `<a href="/docs?a=1&amp;b=2">`}
	got := string(b.Rich("es", "page.rich", nil, slots))
	want := `Lee el <a href="/docs?a=1&amp;b=2" data-i18n-slot="link">protocolo</a> y &lt;notas&gt;.`
	if got != want {
		t.Fatalf("Rich = %s\nwant  %s", got, want)
	}
	plain := string(b.Rich("en", "page.rich", nil, map[string]string{"link": "<script>alert(1)</script>"}))
	if plain != "Read the protocol &amp; &lt;notes&gt;." {
		t.Fatalf("malformed slot tag = %s", plain)
	}
	if got := string(b.Rich("en", "js.greet", Args{"name": "<b>"}, nil)); got != "Hello &lt;b&gt;" {
		t.Fatalf("escaped arg = %s", got)
	}
	if got := string(b.Rich("en", "<x>", nil, nil)); got != "&lt;x&gt;" {
		t.Fatalf("unknown key = %s", got)
	}
}

func TestMessagesAndCatalog(t *testing.T) {
	b := loadTest(t, Options{Pseudo: true}, nil)
	msgs := b.Messages("es", "js.")
	if len(msgs) != 1 || msgs["js.greet"] != "Hola {name}" {
		t.Fatalf("Messages = %v", msgs)
	}
	if got := b.Messages("zz", "page.files")["page.files"].(map[string]string); got["$plural"] != "count" || got["one"] != "a file" || got["other"] != "{count} files" {
		t.Fatalf("plural wire = %v", got)
	}
	body, sum := b.Catalog("es")
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded) != 18 || len(sum) != 12 {
		t.Fatalf("Catalog = %d keys, sum %q, %v", len(decoded), sum, err)
	}
	if body, _ := b.Catalog("fr"); body != nil {
		t.Fatal("unloaded locale has a catalog")
	}
	if url := b.CatalogURL("es"); url != "/i18n/es.json?v="+sum {
		t.Fatalf("CatalogURL = %s", url)
	}
}

func TestPseudo(t *testing.T) {
	b := loadTest(t, Options{Pseudo: true}, nil)
	if got := b.T(PseudoLocale, "js.greet", Args{"name": "Ana"}); got != "⟦Ĥéĺĺó Ana ···⟧" {
		t.Fatalf("pseudo = %q", got)
	}
	if got := b.T(PseudoLocale, "page.files", Args{"count": 2}); !strings.HasPrefix(got, "⟦2 ƒíĺéš") {
		t.Fatalf("pseudo plural = %q", got)
	}
	if got := string(b.Rich(PseudoLocale, "page.rich", nil, map[string]string{"link": "<a>"})); !strings.Contains(got, `<a data-i18n-slot="link">þŕóţóçóĺ</a>`) {
		t.Fatalf("pseudo rich = %s", got)
	}
}

func TestFormatAndJSON(t *testing.T) {
	b := loadTest(t, Options{}, nil)
	if got := b.Format("es", Bytes(1536)); got != "1,5 KB" {
		t.Fatalf("Format = %q", got)
	}
	got, err := json.Marshal(Args{"n": Number{Value: 1.5, Digits: 1}, "b": Bytes(2), "d": Duration{Seconds: 60, Style: "short"}})
	if err != nil || string(got) != `{"b":{"bytes":2},"d":{"seconds":60,"style":"short"},"n":{"digits":1,"num":1.5}}` {
		t.Fatalf("json = %s, %v", got, err)
	}
}

func TestPluralFallsBackToOther(t *testing.T) {
	// English has no "many" variant; Spanish millions fall back to "other"
	// when a message omits it.
	cat := strings.Replace(testES, `"countPlural=many": "{count} de archivos", `, ``, 1)
	b := loadTest(t, Options{}, map[string]string{"es.json": cat})
	if got := b.T("es", "page.files", Args{"count": 3_000_000}); got != "3.000.000 archivos" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestParseMessageBadString(t *testing.T) {
	if _, err := parseMessage(json.RawMessage(`"a {b"`)); err == nil {
		t.Fatal("expected a pattern error")
	}
}

func TestLoadReadError(t *testing.T) {
	fsys := testFS(nil)
	fsys["messages/es.json"] = &fstest.MapFile{Mode: 0o200 | fs.ModeDir}
	if _, err := Load(fsys, "messages", Options{}); err != nil {
		t.Fatalf("a directory named like a catalog is skipped: %v", err)
	}
}
