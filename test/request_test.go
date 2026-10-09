package integration_test

import (
	"bytes"
	"crypto/ecdh"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
	"github.com/haukened/gone/v3/internal/httpx"
)

// newRequestServer serves the real router over a temporary store with
// secret requests enabled and no rate limits.
func newRequestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.DefaultAppConfig
	svc := newService(t, &cfg)
	svc.Requests = svc.Store.(app.RequestStore)
	h := httpx.New(svc, cfg.MaxBytes, nil)
	h.MinTTL, h.MaxTTL = cfg.MinTTL, cfg.MaxTTL
	h.Requests = svc
	srv := httptest.NewServer(h.Router())
	t.Cleanup(srv.Close)
	return srv
}

type createdRequest struct {
	ID          string `json:"id"`
	ManageToken string `json:"manage_token"`
	FillToken   string `json:"fill_token"`
}

// reply is a fully read and closed HTTP response.
type reply struct {
	code   int
	header http.Header
	body   []byte
}

func requestDo(t *testing.T, method, url string, body []byte, headers ...string) reply {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{code: resp.StatusCode, header: resp.Header, body: b}
}

func requestCreate(t *testing.T, base string) createdRequest {
	t.Helper()
	resp := requestDo(t, http.MethodPost, base+"/api/request", nil, "X-Gone-TTL", "30m")
	if resp.code != http.StatusCreated {
		t.Fatalf("create = %d", resp.code)
	}
	var c createdRequest
	if err := json.Unmarshal(resp.body, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// requestReply seals plaintext to pub and PUTs it as the request's reply,
// returning the status code.
func requestReply(t *testing.T, base string, c createdRequest, pub, plaintext []byte) int {
	t.Helper()
	nonce, blob, err := envelope.SealV3(pub, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	resp := requestDo(t, http.MethodPut, base+"/api/request/"+c.ID+"/reply", blob,
		"X-Gone-Version", "3", "X-Gone-Nonce", domain.EncodeB64URL(nonce), httpx.HeaderFill, c.FillToken,
		"Content-Length", strconv.Itoa(len(blob)))
	return resp.code
}

func requestState(t *testing.T, base string, c createdRequest) string {
	t.Helper()
	resp := requestDo(t, http.MethodGet, base+"/api/request/"+c.ID+"/status", nil, httpx.HeaderManage, c.ManageToken)
	if resp.code == http.StatusNotFound {
		return "gone"
	}
	var st struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(resp.body, &st); err != nil {
		t.Fatal(err)
	}
	return st.State
}

// TestRequestRoundTrip runs a whole request over HTTP: the requester's key
// stays here, the server only sees ciphertext, and the reply opens once.
func TestRequestRoundTrip(t *testing.T) {
	srv := newRequestServer(t)
	priv, err := envelope.NewRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	c := requestCreate(t, srv.URL)
	link, err := envelope.ParseReplyLink(srv.URL + "/reply/" + c.ID + "#v3:" + domain.EncodeB64URL(priv.PublicKey().Bytes()) + "." + c.FillToken)
	if err != nil {
		t.Fatalf("ParseReplyLink: %v", err)
	}
	if resp := requestDo(t, http.MethodGet, srv.URL+"/api/request/"+c.ID, nil, httpx.HeaderFill, c.FillToken); resp.code != http.StatusOK {
		t.Fatalf("open check = %d", resp.code)
	}
	if got := requestState(t, srv.URL, c); got != "waiting" {
		t.Fatalf("state = %q", got)
	}
	pt, err := envelope.Pack(envelope.Payload{Message: []byte("db password"), Files: []envelope.File{{Name: "k.txt", Type: "text/plain", Data: []byte("key")}}})
	if err != nil {
		t.Fatal(err)
	}
	if code := requestReply(t, srv.URL, c, link.Fragment.PublicKey, pt); code != http.StatusCreated {
		t.Fatalf("reply = %d", code)
	}
	if code := requestReply(t, srv.URL, c, link.Fragment.PublicKey, pt); code != http.StatusNotFound {
		t.Fatalf("second reply = %d, want 404", code)
	}
	if got := requestState(t, srv.URL, c); got != "ready" {
		t.Fatalf("state after reply = %q", got)
	}
	if resp := requestDo(t, http.MethodGet, srv.URL+"/api/secret/"+c.ID, nil); resp.code != http.StatusNotFound {
		t.Fatalf("reply claimable via secret route: %d", resp.code)
	}
	if resp := requestDo(t, http.MethodGet, srv.URL+"/api/request/"+c.ID+"/reply", nil, httpx.HeaderManage, c.FillToken); resp.code != http.StatusNotFound {
		t.Fatalf("reply claimable with the fill token: %d", resp.code)
	}
	requestOpenReply(t, srv.URL, c, priv)
	if got := requestState(t, srv.URL, c); got != "gone" {
		t.Fatalf("state after open = %q", got)
	}
}

// requestOpenReply claims, decrypts, checks and acknowledges the reply.
func requestOpenReply(t *testing.T, base string, c createdRequest, priv *ecdh.PrivateKey) {
	t.Helper()
	resp := requestDo(t, http.MethodGet, base+"/api/request/"+c.ID+"/reply", nil, httpx.HeaderManage, c.ManageToken)
	if resp.code != http.StatusOK || resp.header.Get("X-Gone-Version") != "3" {
		t.Fatalf("claim = %d version %q", resp.code, resp.header.Get("X-Gone-Version"))
	}
	blob := resp.body
	nonce, err := domain.DecodeB64URL(resp.header.Get("X-Gone-Nonce"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := envelope.OpenV3(priv, nonce, blob)
	if err != nil {
		t.Fatalf("OpenV3: %v", err)
	}
	p, err := envelope.Unpack(pt)
	if err != nil || string(p.Message) != "db password" || len(p.Files) != 1 || string(p.Files[0].Data) != "key" {
		t.Fatalf("payload = %+v, %v", p, err)
	}
	ack := requestDo(t, http.MethodDelete, base+"/api/request/"+c.ID+"/reply", nil, httpx.HeaderClaim, resp.header.Get(httpx.HeaderClaim))
	if ack.code != http.StatusNoContent {
		t.Fatalf("ack = %d", ack.code)
	}
}

func TestRequestCancel(t *testing.T) {
	srv := newRequestServer(t)
	c := requestCreate(t, srv.URL)
	if resp := requestDo(t, http.MethodPost, srv.URL+"/api/request/"+c.ID+"/revoke", nil, httpx.HeaderManage, c.ManageToken); resp.code != http.StatusNoContent {
		t.Fatalf("cancel = %d", resp.code)
	}
	if resp := requestDo(t, http.MethodGet, srv.URL+"/api/request/"+c.ID, nil, httpx.HeaderFill, c.FillToken); resp.code != http.StatusNotFound {
		t.Fatalf("open check after cancel = %d", resp.code)
	}
	priv, _ := envelope.NewRequestKey()
	if code := requestReply(t, srv.URL, c, priv.PublicKey().Bytes(), []byte("x")); code != http.StatusNotFound {
		t.Fatalf("reply after cancel = %d", code)
	}
}
