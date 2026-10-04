package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsurePrivateDir_Errors covers stat and create failures.
func TestEnsurePrivateDir_Errors(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(tmp, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) // #nosec G302 -- restore for TempDir cleanup

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"stat under file", filepath.Join(file, "child"), "stat:"},
		{"create in read-only parent", filepath.Join(locked, "child"), "create:"},
		{"path is a file", file, "not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == "create:" && os.Geteuid() == 0 {
				t.Skip("root bypasses directory permissions")
			}
			err := ensurePrivateDir(tt.dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestNewBlobStorage covers success and a missing root directory.
func TestNewBlobStorage(t *testing.T) {
	if _, err := newBlobStorage(t.TempDir()); err != nil {
		t.Fatalf("newBlobStorage: %v", err)
	}
	_, err := newBlobStorage(filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "init blob storage") {
		t.Fatalf("err = %v, want init blob storage error", err)
	}
}
