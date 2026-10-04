package integration_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/envelope"
)

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

	link := createSecretLink(t, srv.URL, frag, nonce, ct)
	got, token := claimAndOpen(t, srv.URL, link, nonce)
	assertRoundTripPayload(t, got, want)
	assertAckConsumes(t, srv.URL, link, token)
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
			if got := postSecretStatus(t, srv.URL, []byte("ciphertext"), hdr); got != want {
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
		if got := postSecretStatus(t, srv.URL, []byte("ciphertext"), hdr); got != http.StatusBadRequest {
			t.Errorf("duplicate %s: status = %d, want 400", name, got)
		}
	}
}
