package envelope

import (
	"bytes"
	"crypto/ecdh"
	"errors"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
)

func TestSealV3RoundTrip(t *testing.T) {
	priv, err := NewRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	pt := []byte("the database password")
	nonce, blob, err := SealV3(priv.PublicKey().Bytes(), pt)
	if err != nil {
		t.Fatalf("SealV3: %v", err)
	}
	if len(nonce) != domain.NonceSize || len(blob) != domain.V3HeaderSize+len(pt)+domain.TagSize {
		t.Fatalf("nonce %d blob %d", len(nonce), len(blob))
	}
	got, err := OpenV3(priv, nonce, blob)
	if err != nil || !bytes.Equal(got, pt) {
		t.Fatalf("OpenV3 = %q, %v", got, err)
	}
	_, blob2, err := SealV3(priv.PublicKey().Bytes(), pt)
	if err != nil || bytes.Equal(blob[:domain.V3HeaderSize], blob2[:domain.V3HeaderSize]) {
		t.Fatalf("ephemeral key reused: %v", err)
	}
}

func TestSealV3InvalidPublicKey(t *testing.T) {
	for _, pub := range [][]byte{nil, make([]byte, 65), append([]byte{0x04}, make([]byte, 64)...)} {
		if _, _, err := SealV3(pub, []byte("x")); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("SealV3(%x) = %v", pub[:min(len(pub), 4)], err)
		}
	}
}

func TestSealV3NonceReadError(t *testing.T) {
	priv := p256Key(t, seq(1, 32))
	eph := p256Key(t, seq(2, 32))
	if _, _, err := sealV3With(errReader{}, eph, priv.PublicKey().Bytes(), nil); err == nil {
		t.Fatal("want nonce read error")
	}
}

func TestOpenV3Rejects(t *testing.T) {
	priv := p256Key(t, seq(1, 32))
	nonce, blob, err := SealV3(priv.PublicKey().Bytes(), []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	x25519, err := ecdh.X25519().NewPrivateKey(seq(1, 32))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		key   *ecdh.PrivateKey
		nonce []byte
		blob  []byte
	}{
		{"nil key", nil, nonce, blob},
		{"other curve", x25519, nonce, blob},
		{"short nonce", priv, nonce[:11], blob},
		{"short blob", priv, nonce, blob[:domain.V3HeaderSize+domain.TagSize-1]},
	}
	for _, c := range cases {
		if _, err := OpenV3(c.key, c.nonce, c.blob); !errors.Is(err, ErrDecrypt) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestDeriveV3KeyECDHError(t *testing.T) {
	// A low-order X25519 point makes ECDH fail; P-256 has no such points,
	// so this covers the error branch with another curve.
	priv, err := ecdh.X25519().NewPrivateKey(seq(1, 32))
	if err != nil {
		t.Fatal(err)
	}
	peer, err := ecdh.X25519().NewPublicKey(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v3AEAD(priv, peer, nil, nil); err == nil {
		t.Fatal("want ECDH error")
	}
}
