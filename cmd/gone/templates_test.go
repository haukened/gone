package main

import (
	"strings"
	"testing"
	"testing/fstest"
)

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
	tmpls, err := loadTemplatesFrom(validPages())
	if err != nil {
		t.Fatalf("loadTemplatesFrom: %v", err)
	}
	for name, tm := range map[string]interface{ Name() string }{
		"index": tmpls.index, "about": tmpls.about, "secret": tmpls.secret, "error": tmpls.errorPage,
	} {
		if tm.Name() != name {
			t.Fatalf("template %s has name %q", name, tm.Name())
		}
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
			_, err := loadTemplatesFrom(fsys)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}
