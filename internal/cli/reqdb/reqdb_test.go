package reqdb

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func row(id string, created time.Time) Row {
	return Row{
		ID: id, Origin: "https://gone.example", Label: "note " + id[:4], ManageToken: "M",
		PrivateKey: []byte{1, 2, 3}, ReplyLink: "https://gone.example/reply/" + id, State: StateWaiting,
		CreatedAt: created, ExpiresAt: created.Add(time.Hour),
	}
}

func openTest(t *testing.T) (*DB, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "gone")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d, dir
}

func TestOpenIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	_, dir := openTest(t)
	for path, want := range map[string]os.FileMode{dir: dirPerm, filepath.Join(dir, FileName): filePerm} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", path, fi.Mode().Perm(), want)
		}
	}
}

func TestOpenTightensAnExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if fi, _ := os.Stat(path); fi.Mode().Perm() != filePerm {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

func TestPutListResolveDelete(t *testing.T) {
	d, dir := openTest(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	a, b := row("aaaa1111aaaa1111aaaa1111aaaa1111", now), row("aaaa2222aaaa2222aaaa2222aaaa2222", now.Add(time.Minute))
	for _, r := range []Row{a, b} {
		if err := d.Put(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := d.List(ctx, now)
	if err != nil || len(rows) != 2 || rows[0].ID != b.ID || !bytes.Equal(rows[1].PrivateKey, a.PrivateKey) {
		t.Fatalf("List = %+v, %v", rows, err)
	}
	if got, err := d.Resolve(ctx, "AAAA1", now); err != nil || got.ID != a.ID || got.Label != a.Label {
		t.Fatalf("Resolve unique = %+v, %v", got, err)
	}
	var amb *AmbiguousError
	if _, err := d.Resolve(ctx, "aaaa", now); !errors.As(err, &amb) || len(amb.IDs) != 2 || amb.Error() == "" {
		t.Fatalf("Resolve ambiguous err = %v", err)
	}
	if _, err := d.Resolve(ctx, "bbbb", now); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("Resolve none err = %v", err)
	}
	for _, bad := range []string{"aaa", "zzzz", "aaaa-", a.ID + "0"} {
		if _, err := d.Resolve(ctx, bad, now); !errors.Is(err, ErrBadPrefix) {
			t.Errorf("Resolve(%q) err = %v", bad, err)
		}
	}
	if err := d.SetState(ctx, a.ID, StateReady, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Resolve(ctx, a.ID, now); got.State != StateReady || !got.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("after SetState = %+v", got)
	}
	if err := d.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Resolve(ctx, a.ID, now); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("after Delete err = %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if rows, _ := reopened.List(ctx, now); len(rows) != 1 || rows[0].ID != b.ID {
		t.Fatalf("after reopen = %+v", rows)
	}
}

func TestListPrunesExpired(t *testing.T) {
	d, _ := openTest(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	if err := d.Put(ctx, row("cccc1111cccc1111cccc1111cccc1111", now)); err != nil {
		t.Fatal(err)
	}
	if rows, err := d.List(ctx, now.Add(time.Hour)); err != nil || len(rows) != 0 {
		t.Fatalf("List = %+v, %v", rows, err)
	}
	if _, err := d.Resolve(ctx, "cccc", now); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("expired row still resolvable: %v", err)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	d, dir := openTest(t)
	if _, err := d.db.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	if _, err := Open(dir); err == nil {
		t.Fatal("opened a newer schema")
	}
}

func TestOpenErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(file, "gone")); err == nil {
		t.Fatal("opened under a file")
	}
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(filepath.Join(dir, FileName), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("opened a directory as the database")
	}
}

func TestClosedDBErrors(t *testing.T) {
	d, _ := openTest(t)
	_ = d.Close()
	ctx := context.Background()
	if err := d.Put(ctx, row("dddd1111dddd1111dddd1111dddd1111", time.Now())); err == nil {
		t.Error("Put on closed db")
	}
	if _, err := d.List(ctx, time.Now()); err == nil {
		t.Error("List on closed db")
	}
	if _, err := d.Resolve(ctx, "dddd", time.Now()); err == nil {
		t.Error("Resolve on closed db")
	}
	if _, err := d.all(ctx); err == nil {
		t.Error("all on closed db")
	}
}
