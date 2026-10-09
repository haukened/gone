package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// cliRequest makes a request with the CLI and returns its JSON result.
func cliRequest(t *testing.T, r *cliRunner, origin string) (id, link string) {
	t.Helper()
	if code := r.run("", "request", "-s", origin, "--json", "--label", "integration"); code != 0 {
		t.Fatalf("request exit %d: %s", code, r.stderr)
	}
	var got struct{ ID, Link string }
	if err := json.Unmarshal(r.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got.ID, got.Link
}

// TestCLIRequestCLIReply runs request, reply, list and open across two CLI
// users over the real API.
func TestCLIRequestCLIReply(t *testing.T) {
	srv := newTLSServer(t, newRequestHandler(t))
	asker, holder := newCLI(t, srv), newCLI(t, srv)
	id, link := cliRequest(t, asker, srv.URL)
	file := writeFile(t, "id_ed25519", "PRIVATE KEY")
	if code := holder.run("the token", "reply", link, "-f", file); code != 0 {
		t.Fatalf("reply exit %d: %s", code, holder.stderr)
	}
	if code := asker.run("", "request", "list", "--json"); code != 0 || !json.Valid(asker.stdout.Bytes()) {
		t.Fatalf("list exit %d: %s", code, asker.stdout)
	}
	out := t.TempDir()
	if code := asker.run("", "request", "open", id[:6], "-o", out); code != 0 || asker.stdout.String() != "the token" {
		t.Fatalf("open exit %d: %q %s", code, asker.stdout, asker.stderr)
	}
	if b, err := os.ReadFile(filepath.Join(out, "id_ed25519")); err != nil || string(b) != "PRIVATE KEY" {
		t.Fatalf("attachment = %q, %v", b, err)
	}
	if code := asker.run("", "request", "open", id); code != exitNotFound {
		t.Fatalf("second open exit %d", code)
	}
}

// TestInteropCLIRequestBrowserReply answers a CLI request with the reply
// page's own crypto, then opens it with the CLI.
func TestInteropCLIRequestBrowserReply(t *testing.T) {
	requireNode(t)
	srv := newTLSServer(t, newRequestHandler(t))
	asker := newCLI(t, srv)
	id, raw := cliRequest(t, asker, srv.URL)
	link, err := envelope.ParseReplyLink(raw)
	if err != nil {
		t.Fatal(err)
	}
	tc := interopCases[1]
	sealed := bridge(t, jsMsg{Op: "sealReply", Key: domain.EncodeB64URL(link.Fragment.PublicKey), Message: tc.message, Files: tc.files})
	api := apiClient(t, srv.URL, srv.Certificate())
	if _, err := api.Fill(context.Background(), link.ID, link.Fragment.Fill, sealed.Nonce, sealed.Body); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	out := t.TempDir()
	if code := asker.run("", "request", "open", id, "-o", out); code != 0 || asker.stdout.String() != tc.message {
		t.Fatalf("open exit %d: %q %s", code, asker.stdout, asker.stderr)
	}
	assertFilesOnDisk(t, out, tc.files)
}

// TestInteropBrowserRequestCLIReply answers a request whose key the
// browser code made with `gone reply`, then opens it with the browser code.
func TestInteropBrowserRequestCLIReply(t *testing.T) {
	requireNode(t)
	srv := newTLSServer(t, newRequestHandler(t))
	api := apiClient(t, srv.URL, srv.Certificate())
	key := bridge(t, jsMsg{Op: "requestKey"})
	pub, err := base64.RawURLEncoding.DecodeString(key.Key)
	if err != nil {
		t.Fatal(err)
	}
	created, err := api.CreateRequest(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := envelope.NewReplyFragment(pub, created.FillToken.String())
	if err != nil {
		t.Fatal(err)
	}
	link := envelope.ReplyLink{Origin: api.Origin(), ID: created.ID, Fragment: frag}.String()
	holder := newCLI(t, srv)
	if code := holder.run("from the cli", "reply", "-", "--json"); code == 0 {
		t.Fatal("reply without a link on stdin succeeded")
	}
	holder.stdin.Reset()
	if code := holder.run(link+"\n", "reply", "-", "--message-file", writeFile(t, "msg", "from the cli")); code != 0 {
		t.Fatalf("reply exit %d: %s", code, holder.stderr)
	}
	claimed, err := api.ClaimReply(context.Background(), created.ID, created.ManageToken)
	if err != nil {
		t.Fatalf("ClaimReply: %v", err)
	}
	got := bridge(t, jsMsg{Op: "openReply", Key: key.Key, Private: key.Private, Nonce: claimed.Nonce, Body: claimed.Body})
	if got.Message != "from the cli" {
		t.Fatalf("browser opened %q", got.Message)
	}
}
