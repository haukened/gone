package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/cli/reqdb"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// fastPoll shortens --wait polling for the test.
func fastPoll(t *testing.T) {
	t.Helper()
	old := pollInterval
	pollInterval = 10 * time.Millisecond
	t.Cleanup(func() { pollInterval = old })
}

// requestJSON makes a request and returns its JSON result.
func requestJSON(t *testing.T, te *testEnv, server string, args ...string) map[string]string {
	t.Helper()
	te.reset()
	if code := te.run(append([]string{"request", "-s", server, "--json"}, args...)...); code != exitOK {
		t.Fatalf("request exit %d: %s", code, te.stderr)
	}
	var got map[string]string
	if err := json.Unmarshal(te.stdout.Bytes(), &got); err != nil {
		t.Fatalf("json %q: %v", te.stdout, err)
	}
	return got
}

// replier is a second CLI user who answers requests.
func replier(t *testing.T, te *testEnv) *testEnv {
	t.Helper()
	r := newTestEnv(t)
	r.RootCAs = te.RootCAs
	return r
}

// replyWith answers link with message and returns the exit code.
func replyWith(r *testEnv, link, message string, args ...string) int {
	r.reset()
	r.stdin.WriteString(message)
	return r.run(append([]string{"reply", link}, args...)...)
}

// savedRows lists the requests saved in te's config directory.
func savedRows(t *testing.T, te *testEnv) []reqdb.Row {
	t.Helper()
	db, err := reqdb.Open(filepath.Join(te.configDir, configDirName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.List(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRequestReplyOpenRoundTrip(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	created := requestJSON(t, te, srv.URL, "--label", "  Staging DB  ", "--ttl", "30m")
	link, id := created["link"], created["id"]
	if !strings.HasPrefix(link, srv.URL+"/reply/"+id+"#v3:") || created["label"] != "Staging DB" || created["expires_at"] == "" {
		t.Fatalf("created = %v", created)
	}
	if _, err := envelope.ParseReplyLink(link); err != nil {
		t.Fatalf("link does not parse: %v", err)
	}
	rows := savedRows(t, te)
	if len(rows) != 1 || rows[0].ID != id || len(rows[0].PrivateKey) != 32 || rows[0].Label != "Staging DB" {
		t.Fatalf("saved rows = %+v", rows)
	}

	te.reset()
	if code := te.run("request", "open", id[:6]); code != exitNotFound || !strings.Contains(te.stderr.String(), "no reply yet") {
		t.Fatalf("open before reply: exit %d %s", code, te.stderr)
	}

	r := replier(t, te)
	attach := writeTemp(t, "note.txt", "extra")
	if code := replyWith(r, link, "hunter2", "-f", attach); code != exitOK || !strings.Contains(r.stdout.String(), "Sent. Only the requester can open it.") {
		t.Fatalf("reply exit %d: %s %s", code, r.stdout, r.stderr)
	}
	if code := replyWith(r, link, "again"); code != exitNotFound || !strings.Contains(r.stderr.String(), "already answered") {
		t.Fatalf("second reply exit %d: %s", code, r.stderr)
	}

	te.reset()
	if code := te.run("request", "list"); code != exitOK || !strings.Contains(te.stdout.String(), "reply ready") || !strings.Contains(te.stdout.String(), "Staging DB") {
		t.Fatalf("list exit %d:\n%s%s", code, te.stdout, te.stderr)
	}

	out := t.TempDir()
	te.reset()
	if code := te.run("request", "open", id[:8], "-o", out); code != exitOK || te.stdout.String() != "hunter2" {
		t.Fatalf("open exit %d: %q %s", code, te.stdout, te.stderr)
	}
	if b, err := os.ReadFile(filepath.Join(out, "note.txt")); err != nil || string(b) != "extra" {
		t.Fatalf("attachment = %q, %v", b, err)
	}
	if rows := savedRows(t, te); len(rows) != 0 {
		t.Fatalf("request not forgotten: %+v", rows)
	}
	te.reset()
	if code := te.run("request", "open", id[:8]); code != exitNotFound || !strings.Contains(te.stderr.String(), "no saved request") {
		t.Fatalf("reopen exit %d: %s", code, te.stderr)
	}
}

func TestRequestWaitJSONTwoLines(t *testing.T) {
	fastPoll(t)
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	r := replier(t, te)
	done := make(chan int)
	go func() { done <- te.run("request", "-s", srv.URL, "--wait", "--json", "--label", "ci") }()
	var link string
	for deadline := time.Now().Add(5 * time.Second); link == "" && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if rows := savedRows(t, te); len(rows) == 1 {
			link = rows[0].ReplyLink
		}
	}
	if link == "" {
		t.Fatal("request never saved")
	}
	if code := replyWith(r, link, "from ci"); code != exitOK {
		t.Fatalf("reply exit %d: %s", code, r.stderr)
	}
	if code := <-done; code != exitOK {
		t.Fatalf("wait exit %d: %s", code, te.stderr)
	}
	lines := strings.Split(strings.TrimSpace(te.stdout.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"link":"`) || !strings.Contains(lines[1], `"message":"from ci"`) {
		t.Fatalf("output lines = %q", lines)
	}
	if te.stderr.Len() != 0 {
		t.Fatalf("json mode wrote to stderr: %q", te.stderr)
	}
}

func TestRequestWaitInterruptKeepsRequest(t *testing.T) {
	fastPoll(t)
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	id := requestJSON(t, te, srv.URL)["id"]
	ctx, cancel := context.WithCancel(context.Background())
	te.reset()
	done := make(chan int)
	go func() { done <- Run(ctx, []string{"request", "open", id, "--wait"}, te.Env) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if code := <-done; code != exitInternal || !strings.Contains(te.stderr.String(), "Resume with: gone request open "+id[:shortID]+" --wait") {
		t.Fatalf("interrupt exit %d: %s", code, te.stderr)
	}
	if rows := savedRows(t, te); len(rows) != 1 {
		t.Fatalf("request lost on interrupt: %+v", rows)
	}
}

func TestRequestCancel(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	created := requestJSON(t, te, srv.URL)
	te.reset()
	if code := te.run("request", "cancel", created["id"][:5]); code != exitOK || te.stdout.String() != "Cancelled.\n" {
		t.Fatalf("cancel exit %d: %s %s", code, te.stdout, te.stderr)
	}
	r := replier(t, te)
	if code := replyWith(r, created["link"], "late"); code != exitNotFound {
		t.Fatalf("reply after cancel exit %d", code)
	}
	if r.stdin.Len() == 0 {
		t.Fatal("reply read stdin for a closed request")
	}
	te.reset()
	created = requestJSON(t, te, srv.URL)
	te.reset()
	if code := te.run("request", "cancel", created["id"], "--json"); code != exitOK || !strings.Contains(te.stdout.String(), `"cancelled":true`) {
		t.Fatalf("json cancel exit %d: %s", code, te.stdout)
	}
}

func TestRequestServerGoneForgetsRow(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	a := requestJSON(t, te, srv.URL)["id"]
	b := requestJSON(t, te, srv.URL)["id"]
	// Cancel b behind the CLI's back: its saved row is now stale.
	db, err := reqdb.Open(filepath.Join(te.configDir, configDirName))
	if err != nil {
		t.Fatal(err)
	}
	row, _ := db.Resolve(context.Background(), b, time.Now())
	_ = db.Close()
	cl, _ := newClient(te.Env, srv.URL, "test", common{timeout: time.Minute})
	if err := cl.CancelRequest(context.Background(), domain.SecretID(row.ID), domain.ManageToken(row.ManageToken)); err != nil {
		t.Fatal(err)
	}
	te.reset()
	if code := te.run("request", "list", "--json"); code != exitOK {
		t.Fatalf("list exit %d: %s", code, te.stderr)
	}
	var listed []listedRequest
	if err := json.Unmarshal(te.stdout.Bytes(), &listed); err != nil || len(listed) != 2 {
		t.Fatalf("list = %s, %v", te.stdout, err)
	}
	states := map[string]string{listed[0].ID: listed[0].State, listed[1].ID: listed[1].State}
	if states[a] != "waiting" || states[b] != "gone" {
		t.Fatalf("states = %v", states)
	}
	if rows := savedRows(t, te); len(rows) != 1 || rows[0].ID != a {
		t.Fatalf("gone row kept: %+v", rows)
	}
	te.reset()
	if code := te.run("request", "cancel", a); code != exitOK {
		t.Fatal("cancel a")
	}
	te.reset()
	if code := te.run("request", "list"); code != exitOK || !strings.Contains(te.stdout.String(), "No requests saved") {
		t.Fatalf("empty list: %s", te.stdout)
	}
}

func TestRequestOpenGoneOnServer(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	id := requestJSON(t, te, srv.URL)["id"]
	db, _ := reqdb.Open(filepath.Join(te.configDir, configDirName))
	row, _ := db.Resolve(context.Background(), id, time.Now())
	_ = db.Close()
	cl, _ := newClient(te.Env, srv.URL, "test", common{timeout: time.Minute})
	_ = cl.CancelRequest(context.Background(), domain.SecretID(row.ID), domain.ManageToken(row.ManageToken))
	for _, sub := range []string{"open", "cancel"} {
		te.reset()
		_ = requestJSON(t, te, srv.URL) // keep the database non-empty
		te.reset()
		code := te.run("request", sub, id)
		if code != exitNotFound {
			t.Fatalf("%s gone exit %d: %s", sub, code, te.stderr)
		}
	}
	if rows := savedRows(t, te); len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
}

func TestRequestOfflineListAndPrefixErrors(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	var ids []string
	for len(ids) < 2 {
		ids = append(ids, requestJSON(t, te, srv.URL, "--label", "\x1b[31mred")["id"])
	}
	te.reset()
	if code := te.run("request", "list", "--offline"); code != exitOK || strings.Contains(te.stdout.String(), "\x1b") {
		t.Fatalf("offline list exit %d: %q", code, te.stdout)
	}
	cases := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"request", "open", "zz"}, exitUsage, "4 to 32"},
		{[]string{"request", "open", "ffffffff"}, exitNotFound, "no saved request"},
		{[]string{"request", "open"}, exitUsage, "exactly one request ID"},
		{[]string{"request", "bogus"}, exitUsage, "unknown subcommand"},
		{[]string{"request", "list", "x"}, exitUsage, "no arguments"},
		{[]string{"request", "--ttl", "0"}, exitUsage, "--ttl must be positive"},
	}
	common := ids[0][:4]
	if strings.HasPrefix(ids[1], common) {
		cases = append(cases, struct {
			args []string
			code int
			want string
		}{[]string{"request", "open", common}, exitUsage, "matches 2 requests"})
	}
	for _, c := range cases {
		te.reset()
		if code := te.run(c.args...); code != c.code || !strings.Contains(te.stderr.String(), c.want) {
			t.Errorf("%v: exit %d %q", c.args, code, te.stderr)
		}
	}
}

func TestRequestAmbiguousPrefix(t *testing.T) {
	te := newTestEnv(t)
	db, err := reqdb.Open(filepath.Join(te.configDir, configDirName))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, id := range []string{"abcd" + strings.Repeat("1", 28), "abcd" + strings.Repeat("2", 28)} {
		if err := db.Put(context.Background(), reqdb.Row{ID: id, Origin: "https://x.example", PrivateKey: []byte{1}, State: reqdb.StateWaiting, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	if code := te.run("request", "cancel", "abcd"); code != exitUsage || !strings.Contains(te.stderr.String(), "abcd1111, abcd2222") {
		t.Fatalf("ambiguous exit %d: %s", code, te.stderr)
	}
}

func TestReplyRejectsBadLinks(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	created := requestJSON(t, te, srv.URL)
	link, _ := envelope.ParseReplyLink(created["link"])
	bad := make([]byte, 65)
	bad[0] = 4
	frag, err := envelope.NewReplyFragment(bad, link.Fragment.Fill.String())
	if err != nil {
		t.Fatal(err)
	}
	damaged := envelope.ReplyLink{Origin: link.Origin, ID: link.ID, Fragment: frag}.String()
	r := replier(t, te)
	cases := []struct {
		link string
		code int
		want string
	}{
		{"https://gone.example/secret/" + strings.Repeat("a", 32) + "#v1:x", exitUsage, "not a valid gone request link"},
		{damaged, exitIntegrity, "request link is damaged"},
	}
	for _, c := range cases {
		if code := replyWith(r, c.link, "x"); code != c.code || !strings.Contains(r.stderr.String(), c.want) {
			t.Errorf("%s: exit %d %s", c.link, code, r.stderr)
		}
	}
	r.reset()
	r.stdin.WriteString(created["link"] + "\n")
	msg := writeTemp(t, "msg.txt", "from a file")
	if code := r.run("reply", "-", "--message-file", msg, "--json"); code != exitOK || !strings.Contains(r.stdout.String(), `"sent":true`) {
		t.Fatalf("reply from stdin link: exit %d %s %s", code, r.stdout, r.stderr)
	}
}

func TestRequestDatabaseUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	if err := os.WriteFile(filepath.Join(te.configDir, configDirName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := te.run("request", "-s", srv.URL); code != exitIO || !strings.Contains(te.stderr.String(), "request database") {
		t.Fatalf("exit %d: %s", code, te.stderr)
	}
	te.reset()
	te.ConfigDir = func() (string, error) { return "", errors.New("no home") }
	if code := te.run("request", "list"); code != exitIO {
		t.Fatalf("no config dir exit %d", code)
	}
}

func TestCleanLabel(t *testing.T) {
	long := strings.Repeat("é", 100)
	if got := cleanLabel(long); len([]rune(got)) != maxLabelRunes {
		t.Fatalf("len = %d", len([]rune(got)))
	}
	if got := cleanLabel(" a\xffb "); got != "a\uFFFDb" {
		t.Fatalf("got %q", got)
	}
	if tableTime("x") != "x" {
		t.Fatal("tableTime passthrough")
	}
}

func TestRequestLooseFileRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	requestJSON(t, te, srv.URL)
	path := filepath.Join(te.configDir, configDirName, reqdb.FileName)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"request", "list"}, {"request", "-s", srv.URL}} {
		te.reset()
		if code := te.run(args...); code != exitIO || !strings.Contains(te.stderr.String(), "chmod 600 "+path) {
			t.Fatalf("%v: exit %d: %s", args, code, te.stderr)
		}
	}
}
