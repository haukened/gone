//go:build !prod

package web

import (
	"io/fs"
	"testing"
)

// TestDevAssetsHavePreviewModes checks the development filesystem serves the
// dev-only preview module, which production builds deliberately leave out.
func TestDevAssetsHavePreviewModes(t *testing.T) {
	if !Dev {
		t.Fatal("Dev must be true in a development build")
	}
	b, err := fs.ReadFile(Assets, "dev/js/preview.js")
	if err != nil {
		t.Fatalf("dev build must serve dev/js/preview.js: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("dev/js/preview.js is empty")
	}
}
