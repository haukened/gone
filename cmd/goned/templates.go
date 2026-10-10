package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"sync"

	"github.com/haukened/gone/v3/internal/httpx"
	"github.com/haukened/gone/v3/internal/i18n"
)

// templates holds the page templates rendered by the HTTP handlers, each
// with one clone per locale, and the catalogs they render from.
type templates struct {
	index, about, secret, manage, request, requestDetail, reply, errorPage httpx.LocalizedTemplate
	bundle                                                                 *i18n.Bundle
}

// pageFiles maps each page template name to its file in the assets FS.
var pageFiles = []struct{ name, file string }{
	{"index", "index.tmpl.html"},
	{"about", "about.tmpl.html"},
	{"secret", "secret.tmpl.html"},
	{"manage", "manage.tmpl.html"},
	{"request", "request.tmpl.html"},
	{"requestDetail", "request-detail.tmpl.html"},
	{"reply", "reply.tmpl.html"},
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

// templateFuncs returns the locale-independent functions available to every
// page template: asset (fingerprinted static URL), version (the running
// release), and the argument builders args, dur, bytes, and json.
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
		"args":    pairArgs,
		"dur":     func(sec int, style string) i18n.Duration { return i18n.Duration{Seconds: int64(sec), Style: style} },
		"bytes":   toBytes,
		"json":    toJSON,
	}
}

// localeFuncs returns the functions that render text in one locale: t, ta,
// rich, fmt, lang, dir, langCode, locales, and i18nData (the page's inline
// catalog for the browser).
//
// Parameters:
//   - b: the loaded catalogs.
//   - tag: the locale these functions render.
//
// Returns:
//   - template.FuncMap: the functions.
func localeFuncs(b *i18n.Bundle, tag string) template.FuncMap {
	loc := b.Locale(tag)
	data := pageI18nData(b, loc)
	return template.FuncMap{
		"t": func(key string, pairs ...any) (string, error) {
			args, err := pairArgs(pairs...)
			return b.T(tag, key, args), err
		},
		"ta": func(key string, args i18n.Args) string { return b.T(tag, key, args) },
		"rich": func(key string, args i18n.Args, slots ...string) (template.HTML, error) {
			if len(slots)%2 != 0 {
				return "", errors.New("rich: slots must be name and tag pairs")
			}
			m := make(map[string]string, len(slots)/2)
			for i := 0; i < len(slots); i += 2 {
				m[slots[i]] = slots[i+1]
			}
			return b.Rich(tag, key, args, m), nil
		},
		"fmt":      func(v any) string { return b.Format(tag, v) },
		"lang":     func() string { return loc.Tag },
		"dir":      func() string { return loc.Dir },
		"langCode": func() string { return strings.ToUpper(strings.SplitN(loc.Tag, "-", 2)[0]) },
		"locales":  b.Locales,
		"i18nData": func() map[string]any { return data },
	}
}

// pageI18nData is the JSON block each page carries for i18n.js: the locale,
// the picker's locales, every catalog's URL, and the js.* and common.*
// messages scripts need before any catalog is fetched.
func pageI18nData(b *i18n.Bundle, loc i18n.Locale) map[string]any {
	urls := map[string]string{}
	for _, l := range b.Locales() {
		urls[l.Tag] = b.CatalogURL(l.Tag)
	}
	return map[string]any{
		"locale":   loc.Tag,
		"dir":      loc.Dir,
		"locales":  b.Locales(),
		"catalogs": urls,
		"messages": b.Messages(loc.Tag, "js.", "common."),
	}
}

// pairArgs builds message arguments from alternating names and values.
//
// Returns:
//   - i18n.Args: the arguments, or nil when pairs is empty.
//   - error: non-nil for an odd count or a non-string name.
func pairArgs(pairs ...any) (i18n.Args, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	if len(pairs)%2 != 0 {
		return nil, errors.New("args: want name and value pairs")
	}
	args := make(i18n.Args, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		name, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("args: name %v is not a string", pairs[i])
		}
		args[name] = pairs[i+1]
	}
	return args, nil
}

// toBytes converts a template integer to an i18n.Bytes size.
func toBytes(n any) (i18n.Bytes, error) {
	switch v := n.(type) {
	case int:
		return i18n.Bytes(v), nil
	case int64:
		return i18n.Bytes(v), nil
	}
	return 0, fmt.Errorf("bytes: %T is not an integer", n)
}

// toJSON encodes v for a data-i18n-* attribute.
func toJSON(v any) (string, error) {
	out, err := json.Marshal(v)
	return string(out), err
}

// parsePage parses the shared partials plus a single page template.
//
// Parameters:
//   - fsys: filesystem holding the page template.
//   - base: the already-read partials template content.
//   - name: the name to assign to the page template.
//   - file: the filename of the page template inside fsys.
//   - funcs: template functions from templateFuncs and localeFuncs.
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

// localize clones a parsed page once per loaded locale, binding each clone
// to that locale's functions.
//
// Parameters:
//   - t: the parsed page, never executed itself.
//   - b: the loaded catalogs.
//
// Returns:
//   - httpx.LocalizedTemplate: locale tag to clone.
//   - error: non-nil if cloning fails.
func localize(t *template.Template, b *i18n.Bundle) (httpx.LocalizedTemplate, error) {
	out := httpx.LocalizedTemplate{}
	for _, l := range b.Locales() {
		c, err := t.Clone()
		if err != nil {
			return nil, err
		}
		out[l.Tag] = c.Funcs(localeFuncs(b, l.Tag))
	}
	return out, nil
}

// loadTemplatesFrom reads the partials from fsys and composes every page
// template listed in pageFiles, cloned once per locale in b.
//
// Parameters:
//   - fsys: filesystem containing partials.tmpl.html, the page templates,
//     and the static assets they reference.
//   - ver: release version shown in the footer.
//   - b: the loaded catalogs.
//
// Returns:
//   - *templates: the parsed page templates.
//   - error: non-nil if any template is missing or fails to parse.
func loadTemplatesFrom(fsys fs.FS, ver string, b *i18n.Bundle) (*templates, error) {
	partials, err := fs.ReadFile(fsys, "partials.tmpl.html")
	if err != nil {
		return nil, err
	}
	funcs := templateFuncs(fsys, ver)
	for name, fn := range localeFuncs(b, i18n.BaseLocale) {
		funcs[name] = fn
	}
	parsed := make(map[string]httpx.LocalizedTemplate, len(pageFiles))
	for _, p := range pageFiles {
		t, err := parsePage(fsys, string(partials), p.name, p.file, funcs)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", p.file, err)
		}
		if parsed[p.name], err = localize(t, b); err != nil {
			return nil, fmt.Errorf("clone %s: %w", p.file, err)
		}
	}
	return &templates{
		index: parsed["index"], about: parsed["about"], secret: parsed["secret"], manage: parsed["manage"],
		request: parsed["request"], requestDetail: parsed["requestDetail"], reply: parsed["reply"],
		errorPage: parsed["error"], bundle: b,
	}, nil
}
