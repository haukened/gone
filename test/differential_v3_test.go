package integration_test

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// v3Nonce is the fixed nonce of the hand-sealed v3 cases.
var v3Nonce = bytes.Repeat([]byte{0x33}, domain.NonceSize)

// v3Keys are the fixed requester and ephemeral keys of the v3 cases.
type v3Keys struct {
	req, eph *ecdh.PrivateKey
}

// newV3Keys builds the fixed key pair for the v3 cases.
//
// Parameters:
//   - t: the test.
//
// Returns the keys.
func newV3Keys(t *testing.T) v3Keys {
	t.Helper()
	req, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{0x11}, 32))
	if err != nil {
		t.Fatal(err)
	}
	eph, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{0x22}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return v3Keys{req: req, eph: eph}
}

// jwk renders the requester key as the JWK the bridge's openReply imports.
//
// Parameters:
//   - t: the test.
//
// Returns the JWK JSON.
func (k v3Keys) jwk(t *testing.T) string {
	t.Helper()
	pub, b64 := k.req.PublicKey().Bytes(), base64.RawURLEncoding.EncodeToString
	out, err := json.Marshal(map[string]string{
		"kty": "EC", "crv": "P-256", "d": b64(k.req.Bytes()), "x": b64(pub[1:33]), "y": b64(pub[33:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// seal encrypts pt to the requester like envelope.SealV3, written out from
// docs/protocol.md §4.3 with header in place of the ephemeral point's
// uncompressed encoding: it binds header into the HKDF info and the AAD as
// a sender encoding the point another way would.
//
// Parameters:
//   - t: the test.
//   - header: the bytes written as pubE.
//   - pt: plaintext.
//
// Returns the blob, sealed under v3Nonce.
func (k v3Keys) seal(t *testing.T, header, pt []byte) []byte {
	t.Helper()
	pubR := k.req.PublicKey()
	z, err := k.eph.ECDH(pubR)
	if err != nil {
		t.Fatal(err)
	}
	info := domain.V3HKDFInfo + string(header) + string(pubR.Bytes())
	key, err := hkdf.Key(sha256.New, z, nil, info, domain.KeySize)
	if err != nil {
		t.Fatal(err)
	}
	return newGCM(t, key).Seal(bytes.Clone(header), v3Nonce, pt, append([]byte(domain.AADv3), header...))
}

// encodings returns the ephemeral point's uncompressed, compressed and
// hybrid (SEC 1 0x06/0x07) encodings.
//
// Returns the three encodings.
func (k v3Keys) encodings() (uncompressed, compressed, hybrid []byte) {
	u := k.eph.PublicKey().Bytes()
	odd := u[64] & 1
	return u, append([]byte{0x02 | odd}, u[1:33]...), withByte(u, 0, 0x06|odd)
}

// open decrypts and decodes a reply with the Go requester.
//
// Parameters:
//   - nonce: the nonce.
//   - blob: the blob.
//
// Returns the verdict.
func (k v3Keys) open(nonce, blob []byte) verdict {
	return goPayload(envelope.OpenV3(k.req, nonce, blob))
}

// opened renders what the Go requester opens a browser-sealed reply as.
//
// Parameters:
//   - nonce: the nonce.
//   - blob: the blob.
//
// Returns the payload rendering, or "unopenable".
func (k v3Keys) opened(nonce, blob []byte) string {
	if v := k.open(nonce, blob); v.ok {
		return v.value
	}
	return "unopenable"
}

// openCase opens blob with OpenV3 and with cryptoV3.js decryptV3.
//
// Parameters:
//   - t: the test.
//   - name: case name.
//   - nonce: the nonce.
//   - blob: the v3 blob.
//
// Returns the case.
func (k v3Keys) openCase(t *testing.T, name string, nonce, blob []byte) diffCase {
	return diffCase{
		name: name,
		goV:  func(*testing.T) verdict { return k.open(nonce, blob) },
		js: diffReq{
			Op: "openReply", Key: base64.RawURLEncoding.EncodeToString(k.req.PublicKey().Bytes()),
			Private: k.jwk(t), Nonce: nonce, Body: blob,
		},
		jsValue: jsPayload,
	}
}

// sealCase seals to pubR with SealV3 and with cryptoV3.js encryptV3. A
// sealed reply's value is what the Go requester opens it as.
//
// Parameters:
//   - name: case name.
//   - pubR: the requester public key given to both senders.
//
// Returns the case.
func (k v3Keys) sealCase(name string, pubR []byte) diffCase {
	const msg = "sealed reply"
	return diffCase{
		name: name,
		goV: func(*testing.T) verdict {
			nonce, blob, err := envelope.SealV3(pubR, []byte(msg))
			if err != nil {
				return goVerdict("", err)
			}
			return k.open(nonce, blob)
		},
		js:      diffReq{Op: "sealReply", Key: base64.RawURLEncoding.EncodeToString(pubR), Message: msg},
		jsValue: func(_ *testing.T, r diffReply) string { return k.opened(r.Nonce, r.Body) },
	}
}

// v3Cases are v3 body and key boundaries (docs/protocol.md §4.3).
//
// Parameters:
//   - t: the test.
//
// Returns the cases.
func v3Cases(t *testing.T) []diffCase {
	k := newV3Keys(t)
	u, c, h := k.encodings()
	blob := k.seal(t, u, []byte("v3 differential"))
	pubR := k.req.PublicKey().Bytes()
	odd := pubR[64] & 1
	return []diffCase{
		k.openCase(t, "pubE uncompressed", v3Nonce, blob),
		k.openCase(t, "body 81 bytes", v3Nonce, k.seal(t, u, nil)),
		k.openCase(t, "body 80 bytes", v3Nonce, blob[:domain.V3HeaderSize+domain.TagSize-1]),
		k.openCase(t, "pubE prefix 0x02 (compressed tag)", v3Nonce, withByte(blob, 0, 0x02)),
		k.openCase(t, "pubE compressed 33 bytes, sealed with it", v3Nonce, k.seal(t, c, []byte("compressed"))),
		k.openCase(t, "pubE hybrid 0x06/0x07, sealed with it", v3Nonce, k.seal(t, h, []byte("hybrid"))),
		k.openCase(t, "pubE point at infinity", v3Nonce, append(make([]byte, domain.V3HeaderSize), blob[domain.V3HeaderSize:]...)),
		k.openCase(t, "pubE X off the curve", v3Nonce, withByte(blob, 1, blob[1]^0x01)),
		k.sealCase("seal pubR valid", pubR),
		k.sealCase("seal pubR X off the curve", withByte(pubR, 1, pubR[1]^0x01)),
		k.sealCase("seal pubR compressed 33 bytes", append([]byte{0x02 | odd}, pubR[1:33]...)),
		k.sealCase("seal pubR hybrid 0x06/0x07", withByte(pubR, 0, 0x06|odd)),
		k.sealCase("seal pubR point at infinity 0x00", []byte{0x00}),
		k.sealCase("seal pubR 65 zero bytes", make([]byte, domain.V3PublicKeySize)),
	}
}

// TestDifferentialV3 feeds the same v3 blobs and requester keys to
// OpenV3/SealV3 and to cryptoV3.js and fails when they disagree.
//
// Parameters:
//   - t: the test.
func TestDifferentialV3(t *testing.T) {
	runDiff(t, v3Cases(t))
}
