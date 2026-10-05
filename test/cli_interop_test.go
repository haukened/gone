package integration_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/client"
	"github.com/haukened/gone/v3/internal/envelope"
)

// jsFile is a file in an interop bridge request or reply.
type jsFile struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data []byte `json:"data"`
}

// jsMsg is the interop bridge's request and reply shape (test/interop/interop.js).
type jsMsg struct {
	Op         string   `json:"op,omitempty"`
	Message    string   `json:"message,omitempty"`
	Files      []jsFile `json:"files,omitempty"`
	Passphrase string   `json:"passphrase,omitempty"`
	Version    uint8    `json:"version,omitempty"`
	Key        string   `json:"key,omitempty"`
	Nonce      []byte   `json:"nonce,omitempty"`
	Body       []byte   `json:"body,omitempty"`
}

// interopCase is one message shape exercised in both directions.
type interopCase struct {
	name       string
	message    string
	files      []jsFile
	passphrase string
}

// interopCases covers v1 text, v1 with attachments and v2 with attachments.
var interopCases = []interopCase{
	{name: "v1 text", message: "hello across implementations ✓"},
	{name: "v1 files", message: "two files", files: []jsFile{
		{Name: "a.txt", Type: "text/plain", Data: []byte("alpha")},
		{Name: "b.bin", Type: "application/octet-stream", Data: []byte{0, 1, 2, 255}},
	}},
	{name: "v2 files", message: "locked", passphrase: "correct horse battery staple", files: []jsFile{
		{Name: "key.pem", Type: "text/plain", Data: []byte("-----BEGIN-----\n")},
	}},
}

// requireNode returns the node binary, skipping the test when it is absent
// unless GONE_REQUIRE_NODE=1 (set in CI) makes that a failure.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - string: path to node.
func requireNode(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("GONE_REQUIRE_NODE") == "1" {
			t.Fatal("node is required (GONE_REQUIRE_NODE=1)")
		}
		t.Skip("node not installed")
	}
	return node
}

// bridge runs one request through the browser code via the interop script.
//
// Parameters:
//   - t: the test.
//   - req: the request.
//
// Returns:
//   - jsMsg: the decoded reply.
func bridge(t *testing.T, req jsMsg) jsMsg {
	t.Helper()
	in, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// #nosec G204 -- fixed script path; node resolved from PATH by the test.
	cmd := exec.CommandContext(ctx, requireNode(t), filepath.Join("interop", "interop.js"))
	cmd.Stdin = bytes.NewReader(in)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("interop %s: %v: %s", req.Op, err, stderr.String())
	}
	var reply jsMsg
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatalf("interop reply %q: %v", out, err)
	}
	return reply
}

// apiClient returns a protocol client that trusts srv.
//
// Parameters:
//   - t: the test.
//   - origin: the server URL.
//   - cert: the server's certificate.
//
// Returns:
//   - *client.Client: the client.
func apiClient(t *testing.T, origin string, cert *x509.Certificate) *client.Client {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	c, err := client.New(origin, client.Options{RootCAs: pool, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestInteropBrowserToCLI seals with the browser code, uploads over the
// real API, and opens with `gone get`.
//
// Parameters:
//   - t: the test.
func TestInteropBrowserToCLI(t *testing.T) {
	requireNode(t)
	srv := newGone(t)
	api := apiClient(t, srv.URL, srv.Certificate())
	r := newCLI(t, srv)
	for _, tc := range interopCases {
		t.Run(tc.name, func(t *testing.T) {
			sealed := bridge(t, jsMsg{Op: "seal", Message: tc.message, Files: tc.files, Passphrase: tc.passphrase})
			link := uploadSealed(t, api, srv.URL, sealed)
			out := t.TempDir()
			args := []string{"get", link, "-o", out}
			if tc.passphrase != "" {
				args = append(args, "--passphrase-file", writeFile(t, "pass", tc.passphrase))
			}
			if code := r.run("", args...); code != 0 {
				t.Fatalf("get exit %d: %s", code, r.stderr)
			}
			if r.stdout.String() != tc.message {
				t.Fatalf("message %q", r.stdout)
			}
			assertFilesOnDisk(t, out, tc.files)
		})
	}
}

// TestInteropCLIToBrowser seals with `gone send`, claims over the real API,
// and opens with the browser code.
//
// Parameters:
//   - t: the test.
func TestInteropCLIToBrowser(t *testing.T) {
	requireNode(t)
	srv := newGone(t)
	api := apiClient(t, srv.URL, srv.Certificate())
	r := newCLI(t, srv)
	for _, tc := range interopCases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--server", srv.URL}
			for _, f := range tc.files {
				args = append(args, "--file", writeFile(t, f.Name, string(f.Data)))
			}
			if tc.passphrase != "" {
				args = append(args, "--passphrase-file", writeFile(t, "pass", tc.passphrase))
			}
			raw, _ := r.send(t, tc.message, args...)
			link, err := envelope.ParseLink(raw)
			if err != nil {
				t.Fatal(err)
			}
			v := link.Fragment.Version()
			claimed, err := api.Claim(context.Background(), link.ID, v)
			if err != nil {
				t.Fatal(err)
			}
			key := base64.RawURLEncoding.EncodeToString(link.Fragment.Key())
			got := bridge(t, jsMsg{Op: "open", Version: v, Key: key, Nonce: claimed.Nonce, Body: claimed.Body, Passphrase: tc.passphrase})
			if err := api.Ack(context.Background(), link.ID, claimed.Token); err != nil {
				t.Fatal(err)
			}
			assertPayload(t, got, tc)
		})
	}
}
