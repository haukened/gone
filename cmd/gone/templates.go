package main

import (
	"fmt"
	"html/template"
	"io/fs"
)

// templates holds the parsed page templates rendered by the HTTP handlers.
type templates struct{ index, about, secret, errorPage *template.Template }

// pageFiles maps each page template name to its file in the assets FS.
var pageFiles = []struct{ name, file string }{
	{"index", "index.tmpl.html"},
	{"about", "about.tmpl.html"},
	{"secret", "secret.tmpl.html"},
	{"error", "error.tmpl.html"},
}

// parsePage parses the shared partials plus a single page template.
//
// Parameters:
//   - fsys: filesystem holding the page template.
//   - base: the already-read partials template content.
//   - name: the name to assign to the page template.
//   - file: the filename of the page template inside fsys.
//
// Returns:
//   - *template.Template: the composed page template.
//   - error: non-nil if the page cannot be read or either template fails to parse.
func parsePage(fsys fs.FS, base, name, file string) (*template.Template, error) {
	pageBytes, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	t, err := template.New("partials").Parse(base)
	if err != nil {
		return nil, err
	}
	return t.New(name).Parse(string(pageBytes))
}

// loadTemplatesFrom reads the partials from fsys and composes every page
// template listed in pageFiles.
//
// Parameters:
//   - fsys: filesystem containing partials.tmpl.html and the page templates.
//
// Returns:
//   - *templates: the parsed page templates.
//   - error: non-nil if any template is missing or fails to parse.
func loadTemplatesFrom(fsys fs.FS) (*templates, error) {
	partials, err := fs.ReadFile(fsys, "partials.tmpl.html")
	if err != nil {
		return nil, err
	}
	parsed := make(map[string]*template.Template, len(pageFiles))
	for _, p := range pageFiles {
		t, err := parsePage(fsys, string(partials), p.name, p.file)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", p.file, err)
		}
		parsed[p.name] = t
	}
	return &templates{index: parsed["index"], about: parsed["about"], secret: parsed["secret"], errorPage: parsed["error"]}, nil
}
