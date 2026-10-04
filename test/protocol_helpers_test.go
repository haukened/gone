package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/envelope"
	"github.com/haukened/gone/internal/httpx"
)

// headerVector is one server_headers.json case.
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

// requireHTTPStatus fails t when got does not match want.
//
// Parameters:
//   - t: the test.
//   - label: the operation being checked.
//   - got: the observed HTTP status code.
//   - want: the expected HTTP status code.
func requireHTTPStatus(t *testing.T, label string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s status = %d, want %d", label, got, want)
	}
}

// requireProtocolNonce decodes and validates the claim response nonce.
//
// Parameters:
//   - t: the test.
//   - resp: the claim response.
//   - want: the nonce sent at creation.
//
// Returns:
//   - []byte: the decoded nonce from the response.
func requireProtocolNonce(t *testing.T, resp *http.Response, want []byte) []byte {
	t.Helper()
	got, err := domain.DecodeB64URL(resp.Header.Get("X-Gone-Nonce"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("X-Gone-Nonce = %q, want %x", resp.Header.Get("X-Gone-Nonce"), want)
	}
	return got
}

// openClaimedPayload decrypts and unpacks a claim response body.
//
// Parameters:
//   - t: the test.
//   - link: the parsed share link.
//   - nonce: the response nonce.
//   - body: the response body.
//
// Returns:
//   - envelope.Payload: the decoded payload.
func openClaimedPayload(t *testing.T, link envelope.Link, nonce []byte, body io.Reader) envelope.Payload {
	t.Helper()
	ct, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	plain, err := envelope.Open(link.Fragment.Key(), nonce, ct)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	p, err := envelope.Unpack(plain)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	return p
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	requireHTTPStatus(t, "claim", resp.StatusCode, http.StatusOK)
	if got := resp.Header.Get("X-Gone-Version"); got != "1" {
		t.Fatalf("X-Gone-Version = %q, want 1", got)
	}
	gotNonce := requireProtocolNonce(t, resp, nonce)
	p := openClaimedPayload(t, link, gotNonce, resp.Body)
	return p, resp.Header.Get(httpx.HeaderClaim)
}

// createSecretLink uploads ciphertext and returns the parsed share link.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - frag: the link fragment.
//   - nonce: the creation nonce.
//   - ct: the ciphertext.
//
// Returns:
//   - envelope.Link: the parsed share link.
func createSecretLink(t *testing.T, base string, frag envelope.Fragment, nonce, ct []byte) envelope.Link {
	t.Helper()
	resp := postSecret(t, base, ct, protocolHeaders(domain.EncodeB64URL(nonce)))
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	requireHTTPStatus(t, "create", resp.StatusCode, http.StatusCreated)
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
	built, err := envelope.NewLink(base, id, frag)
	if err != nil {
		t.Fatalf("new link: %v", err)
	}
	link, err := envelope.ParseLink(built.String())
	if err != nil {
		t.Fatalf("parse link %q: %v", built.String(), err)
	}
	return link
}

// assertRoundTripPayload verifies the decoded payload and sanitized metadata.
//
// Parameters:
//   - t: the test.
//   - got: the decoded payload.
//   - want: the original payload.
func assertRoundTripPayload(t *testing.T, got, want envelope.Payload) {
	t.Helper()
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
}

// assertAckConsumes acknowledges the claim and verifies the secret is gone.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - link: the parsed share link.
//   - token: the claim token.
func assertAckConsumes(t *testing.T, base string, link envelope.Link, token string) {
	t.Helper()
	requireHTTPStatus(t, "ack", ack(t, base, link.ID.String(), token), http.StatusNoContent)
	requireHTTPStatus(t, "read after ack", readSecretStatus(t, base, link.ID.String(), ""), http.StatusNotFound)
}
