package integration_test

import (
	"bytes"
	"net/http"
	"testing"
)

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

// postSecretStatus uploads ciphertext and closes the response body.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - body: ciphertext to upload.
//   - hdr: protocol headers to send.
//
// Returns:
//   - int: the response status code.
func postSecretStatus(t *testing.T, base string, body []byte, hdr http.Header) int {
	t.Helper()
	resp := postSecret(t, base, body, hdr)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	return resp.StatusCode
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
