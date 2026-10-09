//go:build !windows

package reqdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireUnsafe checks err is an *UnsafeError naming path, with a fix
// command when wantFix is set.
func requireUnsafe(t *testing.T, err error, path string, wantFix bool) {
	t.Helper()
	var ue *UnsafeError
	if !errors.As(err, &ue) || ue.Path != path {
		t.Fatalf("err = %v, want UnsafeError for %s", err, path)
	}
	if wantFix != (ue.Fix != "") || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestLooseFileRefused(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []os.FileMode{0o640, 0o604, 0o660, 0o666} {
		d, dir := openTest(t)
		if err := d.Put(ctx, row(hexID(1), time.Now())); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, FileName)
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		_, err := d.List(ctx, time.Now())
		requireUnsafe(t, err, path, true)
		if !strings.Contains(err.Error(), "chmod 600 "+path) {
			t.Fatalf("no fix hint: %v", err)
		}
		// Writes are refused too, and the file is left as it was.
		requireUnsafe(t, d.Put(ctx, row(hexID(2), time.Now())), path, true)
		if fi, _ := os.Stat(path); fi.Mode().Perm() != mode {
			t.Fatalf("mode changed to %v", fi.Mode().Perm())
		}
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
		if rows, err := d.List(ctx, time.Now()); err != nil || len(rows) != 1 {
			t.Fatalf("after chmod 600: %+v, %v", rows, err)
		}
	}
}

func TestLooseDirectoryRefused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0o770, 0o707, 0o777} {
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		_, err := Open(dir)
		requireUnsafe(t, err, dir, true)
	}
	// Readable by others is fine: the file inside is still 0600.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatalf("0755 directory refused: %v", err)
	}
}

func TestSymlinkRefused(t *testing.T) {
	d, dir := openTest(t)
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	_, err := d.List(context.Background(), time.Now())
	requireUnsafe(t, err, path, false)
}

func TestForeignOwnerRefused(t *testing.T) {
	d, dir := openTest(t)
	if err := d.Put(context.Background(), row(hexID(1), time.Now())); err != nil {
		t.Fatal(err)
	}
	old := getuid
	t.Cleanup(func() { getuid = old })
	getuid = func() int { return old() + 1 }
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "another user") {
		t.Fatalf("foreign directory err = %v", err)
	}
	_, err := d.List(context.Background(), time.Now())
	requireUnsafe(t, err, filepath.Join(dir, FileName), false)
}
