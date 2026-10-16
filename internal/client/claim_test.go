package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// claimHeaders returns a valid v1 claim response header set.
//
// Returns the headers.
func claimHeaders() http.Header {
	h := http.Header{}
	h.Set(headerVersion, "1")
	h.Set(headerNonce, testNonce)
	h.Set(headerClaim, testClaim.String())
	h.Set(headerClaimExpires, "2030-01-02T03:04:05Z")
	return h
}

// claimServer returns a client whose server answers claims with hdr and body.
//
// Parameters:
//   - t: test handle.
//   - hdr: response headers.
//   - body: response body.
//
// Returns the client.
func claimServer(t *testing.T, hdr http.Header, body []byte) *Client {
	t.Helper()
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/secret/"+testID.String() {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		for k, vs := range hdr {
			w.Header()[k] = vs
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	})
	return c
}

func TestClaimSuccess(t *testing.T) {
	c := claimServer(t, claimHeaders(), []byte("ciphertext"))
	cl, err := c.Claim(context.Background(), testID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(cl.Body) != "ciphertext" || cl.Token != testClaim || !bytes.Equal(cl.Nonce, make([]byte, 12)) {
		t.Fatalf("claimed = %+v", cl)
	}
	if !cl.Expires.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("expires = %v", cl.Expires)
	}
}

func TestClaimHeaderValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(http.Header)
		want   error
	}{
		{"version mismatch", func(h http.Header) { h.Set(headerVersion, "2") }, ErrVersionMismatch},
		{"version missing", func(h http.Header) { h.Del(headerVersion) }, ErrVersionMismatch},
		{"version repeated", func(h http.Header) { h.Add(headerVersion, "1") }, ErrVersionMismatch},
		{"nonce missing", func(h http.Header) { h.Del(headerNonce) }, ErrIntegrity},
		{"nonce short", func(h http.Header) { h.Set(headerNonce, "AAAA") }, ErrIntegrity},
		{"nonce padded", func(h http.Header) { h.Set(headerNonce, testNonce+"==") }, ErrIntegrity},
		{"nonce repeated", func(h http.Header) { h.Add(headerNonce, testNonce) }, ErrIntegrity},
		{"claim missing", func(h http.Header) { h.Del(headerClaim) }, ErrProtocol},
		{"claim bad", func(h http.Header) { h.Set(headerClaim, "short") }, ErrProtocol},
		{"expiry missing", func(h http.Header) { h.Del(headerClaimExpires) }, ErrProtocol},
		{"expiry bad", func(h http.Header) { h.Set(headerClaimExpires, "tomorrow") }, ErrProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := claimHeaders()
			tc.mutate(h)
			c := claimServer(t, h, []byte("ciphertext"))
			if _, err := c.Claim(context.Background(), testID, 1); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestClaimUnsupportedVersion(t *testing.T) {
	h := claimHeaders()
	h.Set(headerVersion, "9")
	c := claimServer(t, h, nil)
	if _, err := c.Claim(context.Background(), testID, 9); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v", err)
	}
}

func TestClaimTooLarge(t *testing.T) {
	h := claimHeaders()
	c, _ := newTLS(t, func(w http.ResponseWriter, _ *http.Request) {
		for k, vs := range h {
			w.Header()[k] = vs
		}
		w.Header().Set("Content-Length", strconv.Itoa(MaxCiphertext+1))
	})
	if _, err := c.Claim(context.Background(), testID, 1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestClaimStatusAndNetwork(t *testing.T) {
	c, srv := newTLS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	})
	if _, err := c.Claim(context.Background(), testID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	srv.Close()
	if _, err := c.Claim(context.Background(), testID, 1); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
}

func TestClaimTruncatedBody(t *testing.T) {
	h := claimHeaders()
	c, _ := newTLS(t, func(w http.ResponseWriter, _ *http.Request) {
		for k, vs := range h {
			w.Header()[k] = vs
		}
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short")
	})
	_, err := c.Claim(context.Background(), testID, 1)
	if err == nil || !errors.Is(err, ErrNetwork) && !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v", err)
	}
}

// flakyClaimServer truncates the body of every claim without a claim
// token and serves the full body to a retry carrying it.
//
// Parameters:
//   - t: test handle.
//   - h: claim response headers.
//   - tokens: receives each request's X-Gone-Claim value.
//
// Returns the client.
func flakyClaimServer(t *testing.T, h http.Header, tokens *[]string) *Client {
	t.Helper()
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		*tokens = append(*tokens, r.Header.Get(headerClaim))
		for k, vs := range h {
			w.Header()[k] = vs
		}
		w.Header().Set("Content-Length", "10")
		if r.Header.Get(headerClaim) == "" {
			_, _ = io.WriteString(w, "ciph")
			return
		}
		_, _ = io.WriteString(w, "ciphertext")
	})
	return c
}

func TestClaimRetriesInterruptedDownload(t *testing.T) {
	var tokens []string
	c := flakyClaimServer(t, claimHeaders(), &tokens)
	cl, err := c.Claim(context.Background(), testID, 1)
	if err != nil || string(cl.Body) != "ciphertext" {
		t.Fatalf("claim = %q, %v", cl.Body, err)
	}
	if len(tokens) != 2 || tokens[0] != "" || tokens[1] != testClaim.String() {
		t.Fatalf("claim tokens sent = %q", tokens)
	}
}

func TestClaimNoRetryAfterLease(t *testing.T) {
	var tokens []string
	h := claimHeaders()
	h.Set(headerClaimExpires, "2001-01-01T00:00:00Z")
	c := flakyClaimServer(t, h, &tokens)
	if _, err := c.Claim(context.Background(), testID, 1); !errors.Is(err, ErrNetwork) || len(tokens) != 1 {
		t.Fatalf("err = %v after %d requests", err, len(tokens))
	}
}

func TestAck(t *testing.T) {
	var method, tok string
	c, srv := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		method, tok = r.Method, r.Header.Get(headerClaim)
		switch tok {
		case testClaim.String():
		case "gone":
			w.WriteHeader(http.StatusNotFound)
			return
		default:
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Ack(context.Background(), testID, testClaim); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete {
		t.Fatalf("method = %s", method)
	}
	// A 404 means the row is already deleted: still a completed deletion.
	if err := c.Ack(context.Background(), testID, "gone"); err != nil {
		t.Fatalf("404 err = %v", err)
	}
	if err := c.Ack(context.Background(), testID, "wrong"); err == nil {
		t.Fatal("409 was accepted")
	}
	srv.Close()
	if err := c.Ack(context.Background(), testID, testClaim); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
}
