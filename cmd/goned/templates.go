package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"sync"
)

// templates holds the parsed page templates rendered by the HTTP handlers.
type templates struct{ index, about, secret, manage, errorPage *template.Template }

// pageFiles maps each page template name to its file in the assets FS.
var pageFiles = []struct{ name, file string }{
	{"index", "index.tmpl.html"},
	{"about", "about.tmpl.html"},
	{"secret", "secret.tmpl.html"},
	{"manage", "manage.tmpl.html"},
	{"error", "error.tmpl.html"},
}

// assetVersions fingerprints static files so each URL changes whenever the
// file's content does. Browsers and CDNs can then cache assets for a long
// time without ever serving a stale copy after an upgrade.
type assetVersions struct {
	fsys fs.FS
	mu   sync.Mutex
	sums map[string]string
}

// url returns the /static/ URL for name with a ?v= content fingerprint, or
// the bare URL when the file can't be read. Fingerprints are computed once.
//
// Parameters:
//   - name: asset path relative to the assets root, e.g. "css/base.css".
//
// Returns:
//   - string: the URL to reference from a template.
func (a *assetVersions) url(name string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	sum, ok := a.sums[name]
	if !ok {
		if b, err := fs.ReadFile(a.fsys, name); err == nil {
			h := sha256.Sum256(b)
			sum = hex.EncodeToString(h[:])[:12]
		}
		a.sums[name] = sum
	}
	if sum == "" {
		return "/static/" + name
	}
	return "/static/" + name + "?v=" + sum
}

// templateFuncs returns the functions available to every page template:
// asset (fingerprinted static URL) and version (the running release).
//
// Parameters:
//   - fsys: filesystem the static assets are served from.
//   - ver: release version, e.g. "v3.3.0", or "dev".
//
// Returns:
//   - template.FuncMap: the functions.
func templateFuncs(fsys fs.FS, ver string) template.FuncMap {
	assets := &assetVersions{fsys: fsys, sums: map[string]string{}}
	return template.FuncMap{
		"asset":   assets.url,
		"version": func() string { return ver },
	}
}

// parsePage parses the shared partials plus a single page template.
//
// Parameters:
//   - fsys: filesystem holding the page template.
//   - base: the already-read partials template content.
//   - name: the name to assign to the page template.
//   - file: the filename of the page template inside fsys.
//   - funcs: template functions from templateFuncs.
//
// Returns:
//   - *template.Template: the composed page template.
//   - error: non-nil if the page cannot be read or either template fails to parse.
func parsePage(fsys fs.FS, base, name, file string, funcs template.FuncMap) (*template.Template, error) {
	pageBytes, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	t, err := template.New("partials").Funcs(funcs).Parse(base)
	if err != nil {
		return nil, err
	}
	return t.New(name).Parse(string(pageBytes))
}

// loadTemplatesFrom reads the partials from fsys and composes every page
// template listed in pageFiles.
//
// Parameters:
//   - fsys: filesystem containing partials.tmpl.html, the page templates,
//     and the static assets they reference.
//   - ver: release version shown in the footer.
//
// Returns:
//   - *templates: the parsed page templates.
//   - error: non-nil if any template is missing or fails to parse.
func loadTemplatesFrom(fsys fs.FS, ver string) (*templates, error) {
	partials, err := fs.ReadFile(fsys, "partials.tmpl.html")
	if err != nil {
		return nil, err
	}
	funcs := templateFuncs(fsys, ver)
	parsed := make(map[string]*template.Template, len(pageFiles))
	for _, p := range pageFiles {
		t, err := parsePage(fsys, string(partials), p.name, p.file, funcs)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", p.file, err)
		}
		parsed[p.name] = t
	}
	return &templates{index: parsed["index"], about: parsed["about"], secret: parsed["secret"], manage: parsed["manage"], errorPage: parsed["error"]}, nil
}
