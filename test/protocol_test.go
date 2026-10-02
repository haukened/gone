package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/envelope"
	"github.com/haukened/gone/internal/httpx"
)

// headerVector is one server_headers.json case.
type headerVector struct {
	In string `json:"in"`
	OK bool   `json:"ok"`
}

// postSecret uploads ciphertext with the given protocol headers. Each header
// value in hdr is added (not set), so repeated values produce duplicates.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - body: ciphertext to upload.
//   - hdr: protocol headers to send.
//
// Returns:
//   - *http.Response: the server's response.
func postSecret(t *testing.T, base string, body []byte, hdr http.Header) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/api/secret", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return do(t, req)
}

// protocolHeaders returns valid v1 create headers for nonce.
//
// Parameters:
//   - nonce: base64url nonce.
//
// Returns:
//   - http.Header: version, nonce and TTL headers.
func protocolHeaders(nonce string) http.Header {
	return http.Header{
		"X-Gone-Version": {"1"},
		"X-Gone-Nonce":   {nonce},
		"X-Gone-Ttl":     {"10m"},
	}
}

// sealPayload packs and encrypts p under a new key.
//
// Parameters:
//   - t: the test.
//   - p: the payload.
//
// Returns:
//   - envelope.Fragment: the link fragment holding the key.
//   - []byte: the nonce.
//   - []byte: the ciphertext.
func sealPayload(t *testing.T, p envelope.Payload) (envelope.Fragment, []byte, []byte) {
	t.Helper()
	plain, err := envelope.Pack(p)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	key, err := envelope.NewKey()
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	nonce, ct, err := envelope.Seal(key, plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	frag, err := envelope.NewFragment(domain.ProtocolV1, key)
	if err != nil {
		t.Fatalf("fragment: %v", err)
	}
	return frag, nonce, ct
}

// claimAndOpen claims the secret named by link, checks the protocol headers
// and decrypts and unpacks the body.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - link: the parsed share link.
//   - nonce: the nonce sent at creation.
//
// Returns:
//   - envelope.Payload: the decoded payload.
//   - string: the claim token.
func claimAndOpen(t *testing.T, base string, link envelope.Link, nonce []byte) (envelope.Payload, string) {
	t.Helper()
	resp := read(t, base, link.ID.String(), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("claim status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Gone-Version"); got != "1" {
		t.Fatalf("X-Gone-Version = %q, want 1", got)
	}
	gotNonce, err := domain.DecodeB64URL(resp.Header.Get("X-Gone-Nonce"))
	if err != nil || !bytes.Equal(gotNonce, nonce) {
		t.Fatalf("X-Gone-Nonce = %q, want %x", resp.Header.Get("X-Gone-Nonce"), nonce)
	}
	ct, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	plain, err := envelope.Open(link.Fragment.Key(), gotNonce, ct)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	p, err := envelope.Unpack(plain)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	return p, resp.Header.Get(httpx.HeaderClaim)
}

func TestProtocolRoundTrip(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	want := envelope.Payload{
		Message: []byte("deploy key below"),
		Files: []envelope.File{
			{Name: "../id_ed25519", Type: "text/plain; charset=utf-8", Data: []byte("-----BEGIN-----")},
			{Name: "notes.bin", Type: "text/html", Data: []byte{0, 1, 2}},
		},
	}
	frag, nonce, ct := sealPayload(t, want)

	resp := postSecret(t, srv.URL, ct, protocolHeaders(domain.EncodeB64URL(nonce)))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	id, err := domain.ParseID(created.ID)
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}
	built, err := envelope.NewLink(srv.URL, id, frag)
	if err != nil {
		t.Fatalf("new link: %v", err)
	}
	link, err := envelope.ParseLink(built.String())
	if err != nil {
		t.Fatalf("parse link %q: %v", built.String(), err)
	}

	got, token := claimAndOpen(t, srv.URL, link, nonce)
	if string(got.Message) != string(want.Message) || len(got.Files) != 2 {
		t.Fatalf("payload = %q with %d files", got.Message, len(got.Files))
	}
	wantFiles := []envelope.File{
		{Name: "id_ed25519", Type: "text/plain", Data: want.Files[0].Data},
		{Name: "notes.bin", Type: envelope.DefaultType, Data: want.Files[1].Data},
	}
	for i, f := range got.Files {
		w := wantFiles[i]
		if f.Name != w.Name || f.Type != w.Type || !bytes.Equal(f.Data, w.Data) {
			t.Fatalf("file %d = %q %q %x, want %q %q %x", i, f.Name, f.Type, f.Data, w.Name, w.Type, w.Data)
		}
	}
	if code := ack(t, srv.URL, link.ID.String(), token); code != http.StatusNoContent {
		t.Fatalf("ack status = %d, want 204", code)
	}
	if code := read(t, srv.URL, link.ID.String(), "").StatusCode; code != http.StatusNotFound {
		t.Fatalf("read after ack status = %d, want 404", code)
	}
}

// loadHeaderVectors reads test/vectors/server_headers.json.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - map[string][]headerVector: cases keyed by "versions" and "nonces".
func loadHeaderVectors(t *testing.T) map[string][]headerVector {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("vectors", "server_headers.json"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var v map[string][]headerVector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}
	return v
}

// TestProtocolHeaderVectors replays the shared header vectors over HTTP.
// Values with surrounding whitespace are skipped: HTTP strips optional
// whitespace before the handler sees them (RFC 9110 §5.5).
func TestProtocolHeaderVectors(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	v := loadHeaderVectors(t)
	check := func(header string, cases []headerVector) {
		for _, c := range cases {
			if strings.TrimSpace(c.In) != c.In {
				continue
			}
			hdr := protocolHeaders("AAAAAAAAAAAAAAAA")
			hdr[header] = []string{c.In}
			want := http.StatusBadRequest
			if c.OK {
				want = http.StatusCreated
			}
			if got := postSecret(t, srv.URL, []byte("ciphertext"), hdr).StatusCode; got != want {
				t.Errorf("%s %q: status = %d, want %d", header, c.In, got, want)
			}
		}
	}
	check("X-Gone-Version", v["versions"])
	check("X-Gone-Nonce", v["nonces"])
}

func TestProtocolDuplicateHeadersRejected(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	for _, name := range []string{"X-Gone-Version", "X-Gone-Nonce", "X-Gone-Ttl"} {
		hdr := protocolHeaders("AAAAAAAAAAAAAAAA")
		hdr[name] = append(hdr[name], hdr[name][0])
		if got := postSecret(t, srv.URL, []byte("ciphertext"), hdr).StatusCode; got != http.StatusBadRequest {
			t.Errorf("duplicate %s: status = %d, want 400", name, got)
		}
	}
}
