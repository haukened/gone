package cli

import (
	"errors"
	"fmt"

	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// sealed is an encrypted payload ready to upload.
type sealed struct {
	version uint8
	key     []byte
	nonce   []byte
	body    []byte
}

// sealPayload packs and encrypts p with a fresh link key: protocol v1 when
// pass is nil, otherwise v2. The caller must clear the returned key.
//
// Parameters:
//   - p: payload to encrypt.
//   - pass: passphrase, or nil.
//
// Returns the sealed payload, or a usage error or internal error.
func sealPayload(p envelope.Payload, pass []byte) (sealed, error) {
	packed, err := envelope.Pack(p)
	if err != nil {
		return sealed{}, fmt.Errorf("encode payload: %w", err)
	}
	defer clear(packed)
	key, err := envelope.NewKey()
	if err != nil {
		return sealed{}, fmt.Errorf("generate key: %w", err)
	}
	s := sealed{version: domain.ProtocolV1, key: key}
	if pass == nil {
		s.nonce, s.body, err = envelope.Seal(key, packed)
	} else {
		s.version = domain.ProtocolV2
		s.nonce, s.body, err = envelope.SealV2(key, string(pass), packed)
	}
	if errors.Is(err, envelope.ErrInvalidPassphrase) {
		err = usagef("the passphrase must be 1 to %d bytes of UTF-8", domain.V2MaxPassphraseBytes)
	}
	if err != nil {
		clear(key)
		return sealed{}, err
	}
	return s, nil
}

// buildLinks composes the share link and manage link for a stored secret.
//
// Parameters:
//   - origin: server origin.
//   - id: secret ID.
//   - s: sealed payload (for version and key).
//   - token: manage token.
//
// Returns the two links or an internal error.
func buildLinks(origin string, id domain.SecretID, s sealed, token domain.ManageToken) (string, string, error) {
	frag, err := envelope.NewFragment(s.version, s.key)
	if err != nil {
		return "", "", fmt.Errorf("build link: %w", err)
	}
	link, err := envelope.NewLink(origin, id, frag)
	if err != nil {
		return "", "", fmt.Errorf("build link: %w", err)
	}
	ml, err := envelope.NewManageLink(origin, id, token.String())
	if err != nil {
		return "", "", fmt.Errorf("build manage link: %w", err)
	}
	return link.String(), ml.String(), nil
}
