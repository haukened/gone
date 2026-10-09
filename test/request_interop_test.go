package integration_test

import (
	"crypto/ecdh"
	"encoding/base64"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// interopPayload packs a case as the browser and CLI both do.
func interopPayload(t *testing.T, tc interopCase) []byte {
	t.Helper()
	p := envelope.Payload{Message: []byte(tc.message)}
	for _, f := range tc.files {
		p.Files = append(p.Files, envelope.File{Name: f.Name, Type: f.Type, Data: f.Data})
	}
	pt, err := envelope.Pack(p)
	if err != nil {
		t.Fatal(err)
	}
	return pt
}

// TestInteropReplyBrowserToGo seals a v3 reply with the reply page's code
// and opens it with internal/envelope.
func TestInteropReplyBrowserToGo(t *testing.T) {
	requireNode(t)
	priv, err := envelope.NewRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	for _, tc := range interopCases[:2] {
		t.Run(tc.name, func(t *testing.T) {
			sealed := bridge(t, jsMsg{Op: "sealReply", Key: pub, Message: tc.message, Files: tc.files})
			pt, err := envelope.OpenV3(priv, sealed.Nonce, sealed.Body)
			if err != nil {
				t.Fatalf("OpenV3: %v", err)
			}
			p, err := envelope.Unpack(pt)
			if err != nil {
				t.Fatal(err)
			}
			got := jsMsg{Message: string(p.Message)}
			for _, f := range p.Files {
				got.Files = append(got.Files, jsFile{Name: f.Name, Type: f.Type, Data: f.Data})
			}
			assertPayload(t, got, tc)
		})
	}
}

// TestInteropReplyGoToBrowser seals a v3 reply with internal/envelope to a
// key made by the browser code, and opens it with the request page's code.
func TestInteropReplyGoToBrowser(t *testing.T) {
	requireNode(t)
	key := bridge(t, jsMsg{Op: "requestKey"})
	raw, err := base64.RawURLEncoding.DecodeString(key.Key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		t.Fatalf("browser public key: %v", err)
	}
	for _, tc := range interopCases[:2] {
		t.Run(tc.name, func(t *testing.T) {
			nonce, blob, err := envelope.SealV3(raw, interopPayload(t, tc))
			if err != nil {
				t.Fatal(err)
			}
			got := bridge(t, jsMsg{Op: "openReply", Key: key.Key, Private: key.Private, Nonce: nonce, Body: blob})
			assertPayload(t, got, tc)
		})
	}
}
