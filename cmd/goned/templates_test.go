package main

import (
	"html/template"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/haukened/gone/v3/internal/httpx"
	"github.com/haukened/gone/v3/internal/i18n"
	wembed "github.com/haukened/gone/v3/web"
)

// testBundle loads the real message catalogs, with the pseudo-locale.
//
// Parameters:
//   - t: the test handle.
//
// Returns:
//   - *i18n.Bundle: the catalogs.
func testBundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b, err := i18n.Load(wembed.Assets, "messages", i18n.Options{Pseudo: true})
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}
	return b
}

// validPages returns a MapFS containing partials and every page template.
func validPages() fstest.MapFS {
	fsys := fstest.MapFS{"partials.tmpl.html": {Data: []byte(`{{define "p"}}x{{end}}`)}}
	for _, p := range pageFiles {
		fsys[p.file] = &fstest.MapFile{Data: []byte(`{{template "p"}}` + p.name)}
	}
	return fsys
}

// TestLoadTemplatesFrom_Success verifies each page is parsed and assigned.
func TestLoadTemplatesFrom_Success(t *testing.T) {
	b := testBundle(t)
	tmpls, err := loadTemplatesFrom(validPages(), "v0.0.0-test", b)
	if err != nil {
		t.Fatalf("loadTemplatesFrom: %v", err)
	}
	for name, lt := range map[string]httpx.LocalizedTemplate{
		"index": tmpls.index, "about": tmpls.about, "secret": tmpls.secret, "manage": tmpls.manage, "error": tmpls.errorPage,
	} {
		if len(lt) != len(b.Locales()) {
			t.Fatalf("template %s has %d clones, want %d", name, len(lt), len(b.Locales()))
		}
		for tag, tm := range lt {
			if tm.Name() != name {
				t.Fatalf("template %s/%s has name %q", name, tag, tm.Name())
			}
		}
	}
	if tmpls.bundle != b {
		t.Fatal("bundle not kept")
	}
}

// TestLoadTemplatesFrom_PageErrors covers missing pages and bad syntax.
func TestLoadTemplatesFrom_PageErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(fstest.MapFS)
		want   string
	}{
		{"missing page", func(f fstest.MapFS) { delete(f, "secret.tmpl.html") }, "parse secret.tmpl.html"},
		{"bad page syntax", func(f fstest.MapFS) { f["about.tmpl.html"].Data = []byte("{{") }, "parse about.tmpl.html"},
		{"bad partials syntax", func(f fstest.MapFS) { f["partials.tmpl.html"].Data = []byte("{{end}}") }, "parse index.tmpl.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := validPages()
			tt.mutate(fsys)
			_, err := loadTemplatesFrom(fsys, "v0.0.0-test", testBundle(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestTemplateFuncs verifies asset URLs carry a content fingerprint that
// changes with the file, and that version reports the build.
func TestTemplateFuncs(t *testing.T) {
	fsys := fstest.MapFS{"css/a.css": {Data: []byte("a{}")}}
	asset := templateFuncs(fsys, "v1.2.3")["asset"].(func(string) string)
	first := asset("css/a.css")
	if !strings.HasPrefix(first, "/static/css/a.css?v=") || len(first) != len("/static/css/a.css?v=")+12 {
		t.Fatalf("asset url = %q", first)
	}
	if again := asset("css/a.css"); again != first {
		t.Fatalf("fingerprint not stable: %q then %q", first, again)
	}
	if got := asset("css/missing.css"); got != "/static/css/missing.css" {
		t.Fatalf("missing asset url = %q", got)
	}
	changed := templateFuncs(fstest.MapFS{"css/a.css": {Data: []byte("b{}")}}, "v1.2.3")["asset"].(func(string) string)
	if changed("css/a.css") == first {
		t.Fatal("fingerprint did not change with content")
	}
	if v := templateFuncs(fsys, "v1.2.3")["version"].(func() string)(); v != "v1.2.3" {
		t.Fatalf("version = %q", v)
	}
}

// TestTemplateArgHelpers covers the argument builders' error paths.
func TestTemplateArgHelpers(t *testing.T) {
	if args, err := pairArgs(); args != nil || err != nil {
		t.Fatalf("empty = %v, %v", args, err)
	}
	if _, err := pairArgs("a"); err == nil {
		t.Fatal("odd pairs accepted")
	}
	if _, err := pairArgs(1, 2); err == nil {
		t.Fatal("non-string name accepted")
	}
	if b, err := toBytes(int64(5)); b != 5 || err != nil {
		t.Fatalf("int64 = %v, %v", b, err)
	}
	if _, err := toBytes("5"); err == nil {
		t.Fatal("string accepted as bytes")
	}
	funcs := localeFuncs(testBundle(t), "es")
	rich := funcs["rich"].(func(string, i18n.Args, ...string) (template.HTML, error))
	if _, err := rich("send.title", nil, "accent"); err == nil {
		t.Fatal("odd slot list accepted")
	}
	tf := funcs["t"].(func(string, ...any) (string, error))
	if _, err := tf("send.title", "x"); err == nil {
		t.Fatal("odd t args accepted")
	}
	if code := funcs["langCode"].(func() string)(); code != "ES" {
		t.Fatalf("langCode = %q", code)
	}
}
