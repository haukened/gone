package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/store/sqlite"
	wembed "github.com/haukened/gone/web"
)

var (
	idAttr     = regexp.MustCompile(`\sid="([^"]+)"`)
	refAttr    = regexp.MustCompile(`\s(?:for|aria-labelledby|aria-describedby|aria-controls)="([^"]+)"`)
	useHref    = regexp.MustCompile(`<use href="#([^"]+)"`)
	assetRef   = regexp.MustCompile(`\s(?:src|href)="(/static/[^"]+)"`)
	inlineCode = regexp.MustCompile(`(?i)<script>|<style|\sstyle="|\son[a-z]+="|javascript:`)
)

// pageRouter builds the production router backed by the real embedded
// templates and assets, with a temporary SQLite index.
//
// Parameters:
//   - t: the calling test; resources are released via t.Cleanup.
//
// Returns:
//   - http.Handler: the fully wired router.
func pageRouter(t *testing.T) http.Handler {
	t.Helper()
	tmp := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(tmp, "gone.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	idx, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("sqlite init: %v", err)
	}
	tmpls, err := loadTemplatesFrom(wembed.Assets)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	cfg := &config.Config{MaxBytes: 2048, MinTTL: time.Minute, MaxTTL: time.Hour, TTLOptions: []domain.TTLOption{
		{Duration: time.Minute, Label: "1m"}, {Duration: time.Hour, Label: "1h"},
	}}
	svc := buildService(idx, stubBlobStorage{}, cfg, realClock{})
	return buildHandler(cfg, svc, db, tmp, tmpls, wembed.Assets).Router()
}

// get issues a GET against h and returns the status code and body.
//
// Parameters:
//   - h: the handler under test.
//   - path: the request path.
//
// Returns:
//   - int: the response status code.
//   - string: the response body.
func get(h http.Handler, path string) (int, string) {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr.Code, rr.Body.String()
}

// idSet returns every id declared in body, failing the test on duplicates.
//
// Parameters:
//   - t: the calling test.
//   - body: the rendered HTML.
//
// Returns:
//   - map[string]bool: the set of declared ids.
func idSet(t *testing.T, body string) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, m := range idAttr.FindAllStringSubmatch(body, -1) {
		if ids[m[1]] {
			t.Errorf("duplicate id %q", m[1])
		}
		ids[m[1]] = true
	}
	return ids
}

// TestPages_DOMContract renders every page through the real router and checks
// the ids the JavaScript modules depend on, that every label/ARIA/icon
// reference resolves, that no inline code would violate the CSP, and that
// every referenced static asset is served.
func TestPages_DOMContract(t *testing.T) {
	h := pageRouter(t)
	shared := []string{"main", "theme-toggle", "i-mark"}
	pages := []struct {
		path   string
		status int
		ids    []string
	}{
		{"/", http.StatusOK, []string{
			"create-secret", "compose", "secret", "drop-zone", "secret-files", "file-list", "size-box",
			"size-label", "size-meter", "size-warning", "size-warning-text", "upload-progress",
			"submit-error", "submit-error-content", "result", "result-heading", "share-link",
			"copy-link", "copy-status", "result-expiry",
		}},
		{"/secret/" + strings.Repeat("a", 32), http.StatusOK, []string{
			"view-open", "open-secret", "download-progress", "consume-status", "consume-error",
			"consume-error-text", "view-revealed", "revealed-heading", "ack-warning", "message-panel",
			"copy-secret", "secret-output", "copy-status", "file-section", "file-output-list",
			"download-all", "view-gone",
		}},
		{"/about", http.StatusOK, nil},
		{"/does-not-exist", http.StatusNotFound, nil},
	}
	assets := map[string]bool{}
	for _, p := range pages {
		t.Run(p.path, func(t *testing.T) {
			code, body := get(h, p.path)
			if code != p.status {
				t.Fatalf("status %d, want %d", code, p.status)
			}
			ids := idSet(t, body)
			for _, id := range append(append([]string{}, shared...), p.ids...) {
				if !ids[id] {
					t.Errorf("missing id %q", id)
				}
			}
			for _, m := range refAttr.FindAllStringSubmatch(body, -1) {
				for _, ref := range strings.Fields(m[1]) {
					if !ids[ref] {
						t.Errorf("reference to undeclared id %q", ref)
					}
				}
			}
			for _, m := range useHref.FindAllStringSubmatch(body, -1) {
				if !ids[m[1]] {
					t.Errorf("icon %q is not defined", m[1])
				}
			}
			if loc := inlineCode.FindStringIndex(body); loc != nil {
				t.Errorf("inline code violates CSP: %q", body[loc[0]:min(loc[1]+40, len(body))])
			}
			if !strings.Contains(body, `<html lang="en"`) {
				t.Errorf("missing document language")
			}
			for _, m := range assetRef.FindAllStringSubmatch(body, -1) {
				assets[m[1]] = true
			}
		})
	}
	if len(assets) == 0 {
		t.Fatal("no static assets referenced")
	}
	for path := range assets {
		if code, _ := get(h, path); code != http.StatusOK {
			t.Errorf("asset %s: status %d", path, code)
		}
	}
}
