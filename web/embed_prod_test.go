package web

// These tests require the minified assets be produced by the build process.
// `task minify` will create the necessary files in the `web` directory.
// then run tests with `go test -tags=prod ./web`

import "testing"

func TestProdAssetsOpen(t *testing.T) {
	// This test ensures that the embedded filesystem is accessible and contains expected files.
	tests := []struct {
		name      string
		path      string
		wantError bool
	}{
		{
			name:      "existing file",
			path:      "index.tmpl.html",
			wantError: false,
		},
		{
			name:      "non existent file",
			path:      "this_file_should_not_exist_12345.go",
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Assets.Open(tc.path)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error opening %q, got none", tc.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error opening %q: %v", tc.path, err)
			}
			defer func() {
				if cerr := f.Close(); cerr != nil {
					t.Fatalf("close failed: %v", cerr)
				}
			}()
			// Spot check first few bytes to ensure we read a regular file.
			buf := make([]byte, 16)
			n, rerr := f.Read(buf)
			if rerr != nil && rerr.Error() != "EOF" {
				// Allow EOF if file shorter than 16 bytes (unlikely here).
				t.Fatalf("read failed: %v", rerr)
			}
			if n == 0 {
				t.Fatalf("read zero bytes from %q; expected some content", tc.path)
			}
		})
	}
}
