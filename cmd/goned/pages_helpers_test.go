package main

import (
	"net/http"
	"strings"
	"testing"
)

// checkPageContract verifies one page's DOM contract.
//
// Parameters:
//   - t: the test handle.
//   - h: the handler under test.
//   - p: the page contract case.
//   - assets: accumulator for referenced static assets.
func checkPageContract(t *testing.T, h http.Handler, p pageContractCase, assets map[string]bool) {
	t.Helper()
	code, body := get(h, p.path)
	if code != p.status {
		t.Fatalf("status %d, want %d", code, p.status)
	}
	ids := idSet(t, body)
	requireIDs(t, ids, append(append([]string{}, sharedPageIDs...), p.ids...))
	requireDeclaredRefs(t, ids, body)
	requireDeclaredIcons(t, ids, body)
	requireNoInlineCode(t, body)
	requireDocumentLanguage(t, body)
	requireFingerprintedAssets(t, body)
	collectAssets(body, assets)
}

// requireIDs verifies expected element ids are declared.
//
// Parameters:
//   - t: the test handle.
//   - ids: declared ids keyed by name.
//   - want: required ids.
func requireIDs(t *testing.T, ids map[string]bool, want []string) {
	t.Helper()
	for _, id := range want {
		if !ids[id] {
			t.Errorf("missing id %q", id)
		}
	}
}

// requireDeclaredRefs verifies label and ARIA references resolve.
//
// Parameters:
//   - t: the test handle.
//   - ids: declared ids keyed by name.
//   - body: rendered HTML.
func requireDeclaredRefs(t *testing.T, ids map[string]bool, body string) {
	t.Helper()
	for _, m := range refAttr.FindAllStringSubmatch(body, -1) {
		checkRefFields(t, ids, m[1])
	}
}

// checkRefFields verifies all whitespace-separated references resolve.
//
// Parameters:
//   - t: the test handle.
//   - ids: declared ids keyed by name.
//   - refs: whitespace-separated references.
func checkRefFields(t *testing.T, ids map[string]bool, refs string) {
	t.Helper()
	for _, ref := range strings.Fields(refs) {
		if !ids[ref] {
			t.Errorf("reference to undeclared id %q", ref)
		}
	}
}

// requireDeclaredIcons verifies SVG use references resolve.
//
// Parameters:
//   - t: the test handle.
//   - ids: declared ids keyed by name.
//   - body: rendered HTML.
func requireDeclaredIcons(t *testing.T, ids map[string]bool, body string) {
	t.Helper()
	for _, m := range useHref.FindAllStringSubmatch(body, -1) {
		if !ids[m[1]] {
			t.Errorf("icon %q is not defined", m[1])
		}
	}
}

// requireNoInlineCode verifies the page would satisfy the CSP.
//
// Parameters:
//   - t: the test handle.
//   - body: rendered HTML.
func requireNoInlineCode(t *testing.T, body string) {
	t.Helper()
	if loc := inlineCode.FindStringIndex(body); loc != nil {
		t.Errorf("inline code violates CSP: %q", body[loc[0]:min(loc[1]+40, len(body))])
	}
}

// requireDocumentLanguage verifies the root document language.
//
// Parameters:
//   - t: the test handle.
//   - body: rendered HTML.
func requireDocumentLanguage(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, `<html lang="en"`) {
		t.Errorf("missing document language")
	}
}

// collectAssets adds referenced static assets to assets.
//
// Parameters:
//   - body: rendered HTML.
//   - assets: accumulator keyed by asset path.
func collectAssets(body string, assets map[string]bool) {
	for _, m := range assetRef.FindAllStringSubmatch(body, -1) {
		assets[m[1]] = true
	}
}

// requireFingerprintedAssets fails when a static asset is referenced without
// a ?v= content fingerprint, which would let caches serve it stale.
//
// Parameters:
//   - t: the test handle.
//   - body: rendered HTML.
func requireFingerprintedAssets(t *testing.T, body string) {
	t.Helper()
	for _, m := range assetRef.FindAllStringSubmatch(body, -1) {
		if !strings.Contains(m[1], "?v=") {
			t.Errorf("asset %s has no fingerprint", m[1])
		}
	}
	if !strings.Contains(body, "Gone v0.0.0-test") {
		t.Errorf("footer is missing the version")
	}
}

// checkReferencedAssets verifies all collected static assets are served.
//
// Parameters:
//   - t: the test handle.
//   - h: the handler under test.
//   - assets: collected asset paths.
func checkReferencedAssets(t *testing.T, h http.Handler, assets map[string]bool) {
	t.Helper()
	if len(assets) == 0 {
		t.Fatal("no static assets referenced")
	}
	for path := range assets {
		if code, _ := get(h, path); code != http.StatusOK {
			t.Errorf("asset %s: status %d", path, code)
		}
	}
}
