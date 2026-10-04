package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/client"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/envelope"
)

// TestSealPayloadVersions checks that a passphrase selects protocol v2 and
// that the result opens with the same key.
//
// Parameters:
//   - t: the test.
func TestSealPayloadVersions(t *testing.T) {
	tests := []struct {
		name string
		pass string
		want uint8
	}{
		{"v1", "", domain.ProtocolV1},
		{"v2", "correct horse", domain.ProtocolV2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := sealPayloadForTest(t, tt.pass)
			if err != nil || s.version != tt.want {
				t.Fatalf("seal: %+v %v", s, err)
			}
			if pt, err := openSealed(s.sealed, tt.pass); err != nil || string(pt) != "secret" {
				t.Fatalf("open: %q %v", pt, err)
			}
		})
	}
}

// openSealed opens s with the scheme implied by pass.
//
// Parameters:
//   - s: sealed payload.
//   - pass: passphrase, or "" for v1.
//
// Returns the plaintext and any error.
func openSealed(s sealed, pass string) ([]byte, error) {
	if pass == "" {
		return envelope.Open(s.key, s.nonce, s.body)
	}
	return envelope.OpenV2(s.key, pass, s.nonce, s.body)
}

// TestSealPayloadErrors checks too many files and an invalid passphrase.
//
// Parameters:
//   - t: the test.
func TestSealPayloadErrors(t *testing.T) {
	many := envelope.Payload{Files: make([]envelope.File, envelope.MaxFiles+1)}
	if _, err := sealPayload(many, nil); !errors.Is(err, envelope.ErrTooManyFiles) {
		t.Fatalf("too many files: %v", err)
	}
	_, err := sealPayload(envelope.Payload{Message: []byte("x")}, []byte{0xff, 0xfe})
	var ue *usageError
	if !errors.As(err, &ue) || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("bad passphrase: %v", err)
	}
}

// TestBuildLinks checks link construction and its error paths.
//
// Parameters:
//   - t: the test.
func TestBuildLinks(t *testing.T) {
	id, tok, key := linkFixtures(t)
	good := sealed{version: domain.ProtocolV1, key: key}
	link, manage, err := buildLinks("https://example.com", id, good, tok)
	if err != nil || !strings.HasPrefix(link, "https://example.com/secret/"+id.String()+"#v1:") ||
		manage != "https://example.com/manage/"+id.String()+"#"+tok.String() {
		t.Fatalf("links = %q %q %v", link, manage, err)
	}
}

// TestBuildLinksErrors checks each invalid input to buildLinks.
//
// Parameters:
//   - t: the test.
func TestBuildLinksErrors(t *testing.T) {
	id, tok, key := linkFixtures(t)
	good := sealed{version: domain.ProtocolV1, key: key}
	tests := []struct {
		name   string
		origin string
		id     domain.SecretID
		s      sealed
		tok    domain.ManageToken
		want   string
	}{
		{"bad version", "https://example.com", id, sealed{version: 9, key: key}, tok, "build link"},
		{"bad origin", "https://example.com/path", id, good, tok, "build link"},
		{"bad id", "https://example.com", domain.SecretID("x"), good, tok, "build link"},
		{"bad token", "https://example.com", id, good, domain.ManageToken(""), "manage link"},
	}
	for _, tt := range tests {
		if _, _, err := buildLinks(tt.origin, tt.id, tt.s, tt.tok); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
}

// linkFixtures returns a fresh secret ID, manage token and key.
//
// Parameters:
//   - t: the test.
//
// Returns the ID, token and key.
func linkFixtures(t *testing.T) (domain.SecretID, domain.ManageToken, []byte) {
	t.Helper()
	id, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	tok, err := domain.NewManageToken()
	if err != nil {
		t.Fatal(err)
	}
	key, err := envelope.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return id, tok, key
}

// testSealed is a sealed payload with a helper to view it as a claim.
type testSealed struct{ sealed }

// claimed returns s as the server would return it from a claim.
//
// Returns the claim.
func (s testSealed) claimed() client.Claimed {
	return client.Claimed{Nonce: s.nonce, Body: s.body}
}

// sealPayloadForTest seals the message "secret" under pass.
//
// Parameters:
//   - t: the test.
//   - pass: passphrase, or "" for v1.
//
// Returns the sealed payload and any error.
func sealPayloadForTest(t *testing.T, pass string) (testSealed, error) {
	t.Helper()
	var p []byte
	if pass != "" {
		p = []byte(pass)
	}
	s, err := sealPayload(envelope.Payload{Message: []byte("secret")}, p)
	return testSealed{s}, err
}
