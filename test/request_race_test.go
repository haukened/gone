package integration_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
	"github.com/haukened/gone/v3/internal/httpx"
)

// requestPut PUTs body as the request's reply without failing the test, so
// it can run in a goroutine.
//
// Parameters:
//   - url: the reply URL.
//   - c: the request.
//   - nonce: base64url nonce header.
//   - body: ciphertext stream.
//   - size: ciphertext length.
//
// Returns: the status code, or 0 if the request failed.
func requestPut(url string, c createdRequest, nonce string, body io.Reader, size int) int {
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return 0
	}
	req.ContentLength = int64(size)
	req.Header.Set("X-Gone-Version", "3")
	req.Header.Set("X-Gone-Nonce", nonce)
	req.Header.Set(httpx.HeaderFill, c.FillToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// waitForFile polls until path exists.
//
// Parameters:
//   - t: the test.
//   - path: file to wait for.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never appeared", filepath.Base(path))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRequestConcurrentRepliesOneWins sends two replies to one request so
// that both pass the open check before either is stored: the first is still
// uploading its blob when the second arrives. Exactly one gets 201 and the
// other 404 (docs/protocol.md §8.6).
func TestRequestConcurrentRepliesOneWins(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(newRequestHandlerAt(t, dir))
	t.Cleanup(srv.Close)
	c := requestCreate(t, srv.URL)
	priv, err := envelope.NewRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	external := bytes.Repeat([]byte("x"), int(config.DefaultAppConfig.InlineMaxBytes))
	nonce, blob, err := envelope.SealV3(priv.PublicKey().Bytes(), external)
	if err != nil {
		t.Fatal(err)
	}
	url, nonceB64 := srv.URL+"/api/request/"+c.ID+"/reply", domain.EncodeB64URL(nonce)
	pr, pw := io.Pipe()
	first := make(chan int, 1)
	go func() { first <- requestPut(url, c, nonceB64, pr, len(blob)) }()
	waitForFile(t, filepath.Join(dir, "blobs", c.ID+".blob"))
	second := requestReply(t, srv.URL, c, priv.PublicKey().Bytes(), external)
	_, _ = pw.Write(blob)
	_ = pw.Close()
	if got := []int{<-first, second}; got[0] != http.StatusCreated || got[1] != http.StatusNotFound {
		t.Fatalf("codes (first, second) = %v, want [201 404]", got)
	}
	if got := requestState(t, srv.URL, c); got != "ready" {
		t.Fatalf("state = %q, want ready", got)
	}
}
