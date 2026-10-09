package reqdb

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// hexID returns a 32-hex ID from n.
func hexID(n int) string { return fmt.Sprintf("%032x", n+1) }

func TestFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	d, dir := openTest(t)
	if err := d.Put(context.Background(), row(hexID(1), time.Now())); err != nil {
		t.Fatal(err)
	}
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

func TestPutListResolveDelete(t *testing.T) {
	d, dir := openTest(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	a, b := row("aaaa1111aaaa1111aaaa1111aaaa1111", now), row("aaaa2222aaaa2222aaaa2222aaaa2222", now.Add(time.Minute))
	for _, r := range []Row{a, b, a} {
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
	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if rows, _ := reopened.List(ctx, now); len(rows) != 1 || rows[0].ID != b.ID {
		t.Fatalf("after reopen = %+v", rows)
	}
}

func TestListPrunesExpiredAndCreatesNothing(t *testing.T) {
	d, dir := openTest(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	if rows, err := d.List(ctx, now); err != nil || len(rows) != 0 {
		t.Fatalf("empty List = %+v, %v", rows, err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("listing created the file: %v", err)
	}
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

// TestConcurrentWritersLoseNothing runs many independent handles (as
// separate gone processes would) adding rows at once: the lock and the
// atomic rewrite must keep every one.
func TestConcurrentWritersLoseNothing(t *testing.T) {
	_, dir := openTest(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	now := time.Now()
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := Open(dir)
			if err == nil {
				err = d.Put(context.Background(), row(hexID(i), now))
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	d, _ := Open(dir)
	if rows, err := d.List(context.Background(), now); err != nil || len(rows) != n {
		t.Fatalf("kept %d of %d rows: %v", len(rows), n, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != FileName {
			t.Errorf("left behind %s", e.Name())
		}
	}
}

func TestLockTimeoutStaleAndCancel(t *testing.T) {
	d, dir := openTest(t)
	oldWait, oldStale := lockWait, staleLock
	t.Cleanup(func() { lockWait, staleLock = oldWait, oldStale })
	lockWait, staleLock = 50*time.Millisecond, time.Hour
	lock := filepath.Join(dir, lockName)
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := d.Put(ctx, row(hexID(1), time.Now())); !errors.Is(err, ErrLocked) {
		t.Fatalf("held lock err = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	lockWait = time.Hour
	if err := d.Put(cancelled, row(hexID(1), time.Now())); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait err = %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(ctx, row(hexID(1), time.Now())); err != nil {
		t.Fatalf("stale lock not cleared: %v", err)
	}
	if _, err := os.Stat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock left behind: %v", err)
	}
}

func TestDamagedFiles(t *testing.T) {
	d, dir := openTest(t)
	path := filepath.Join(dir, FileName)
	ctx := context.Background()
	empty, _ := encode(nil)
	cases := map[string][]byte{
		"garbage":   []byte("not a gob"),
		"too large": bytes.Repeat([]byte{0}, maxFileBytes+1),
	}
	for name, data := range cases {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := d.List(ctx, time.Now()); err == nil {
			t.Errorf("%s: List succeeded", name)
		}
	}
	if err := os.WriteFile(path, empty, 0o600); err != nil {
		t.Fatal(err)
	}
	if rows, err := d.List(ctx, time.Now()); err != nil || len(rows) != 0 {
		t.Fatalf("empty file = %+v, %v", rows, err)
	}
}

func TestNewerFormatRejected(t *testing.T) {
	d, dir := openTest(t)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(file{Version: formatVersion + 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.List(context.Background(), time.Now()); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("newer format err = %v", err)
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
	if _, err := Open(file); err == nil {
		t.Fatal("opened a file as the directory")
	}
	if runtime.GOOS == "windows" {
		return
	}
	dir := filepath.Join(t.TempDir(), "gone")
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, FileName), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := d.List(context.Background(), time.Now()); err == nil {
		t.Fatal("read a directory as the request file")
	}
	if err := os.Remove(filepath.Join(dir, FileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if os.Geteuid() != 0 {
		if err := d.Put(context.Background(), row(hexID(1), time.Now())); err == nil {
			t.Fatal("wrote into a read-only directory")
		}
	}
}
