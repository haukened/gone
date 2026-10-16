//go:build prod

package web

import (
	"io/fs"
	"strings"
	"testing"
)

// TestProdAssetsHaveNoPreviewModes guards the content-spoofing fix: the
// developer preview modes (?preview=...) render attacker-chosen text in the
// real UI, so a production build must not contain them at all. The files
// live under web/dev/, which `task minify` never copies, and no shipped
// script may read a preview query parameter.
func TestProdAssetsHaveNoPreviewModes(t *testing.T) {
	if Dev {
		t.Fatal("Dev must be false in a production build")
	}
	files := 0
	err := fs.WalkDir(Assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == "dev" {
				t.Errorf("production assets contain the dev-only directory %q", path)
			}
			return nil
		}
		files++
		if strings.Contains(strings.ToLower(path), "preview") {
			t.Errorf("production assets contain %q", path)
		}
		if strings.HasPrefix(path, "dev/") {
			t.Errorf("production assets contain dev-only file %q", path)
		}
		if strings.HasSuffix(path, ".js") {
			assertNoPreviewParam(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no production assets found; run task minify first")
	}
}

// assertNoPreviewParam fails when the script at path reads ?preview.
func assertNoPreviewParam(t *testing.T, path string) {
	t.Helper()
	b, err := fs.ReadFile(Assets, path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, bad := range []string{`"preview"`, `'preview'`, "preview=", "gonePreview=", "gonePreview ="} {
		if strings.Contains(src, bad) {
			t.Errorf("%s contains %q", path, bad)
		}
	}
}
