package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

var (
	testID     = domain.SecretID("0123456789abcdef0123456789abcdef")
	testManage = domain.ManageToken(strings.Repeat("M", 43))
	testClaim  = domain.ClaimToken(strings.Repeat("Q", 43))
	testNonce  = "AAAAAAAAAAAAAAAA"
)

func TestFixturesValid(t *testing.T) {
	if _, err := domain.ParseID(testID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ParseManageToken(testManage.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ParseClaimToken(testClaim.String()); err != nil {
		t.Fatal(err)
	}
}

// createCapture records what the fake server received for a create.
type createCapture struct {
	method, path, ver, nonce, ttl, ctype string
	body                                 string
	cl                                   int64
}

// check compares the captured create request with the expected v2 upload.
//
// Parameters:
//   - t: the test.
func (got createCapture) check(t *testing.T) {
	t.Helper()
	want := createCapture{
		method: http.MethodPost, path: "/api/secret", ver: "2", nonce: testNonce, ttl: "1h0m0s",
		ctype: "application/octet-stream", body: "cipher", cl: 6,
	}
	if got != want {
		t.Fatalf("request = %+v, want %+v", got, want)
	}
}

// TestCreateSuccess checks the create request headers and body and the
// parsed result.
//
// Parameters:
//   - t: the test.
func TestCreateSuccess(t *testing.T) {
	var got createCapture
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method, got.path = r.Method, r.URL.Path
		got.ver, got.nonce, got.ttl = r.Header.Get("X-Gone-Version"), r.Header.Get("X-Gone-Nonce"), r.Header.Get("X-Gone-TTL")
		got.ctype, got.body, got.cl = r.Header.Get("Content-Type"), string(b), r.ContentLength
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"`+testID.String()+`","expires_at":"2030-01-02T03:04:05Z","manage_token":"`+testManage.String()+`","extra":1}`)
	})
	res, err := c.Create(context.Background(), CreateRequest{Version: 2, Nonce: make([]byte, 12), TTL: time.Hour, Body: []byte("cipher")})
	if err != nil {
		t.Fatal(err)
	}
	got.check(t)
	if res.ID != testID || res.ManageToken != testManage || !res.ExpiresAt.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("result = %+v", res)
	}
}

func TestCreateNilBody(t *testing.T) {
	var cl int64 = -2
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		cl = r.ContentLength
		w.WriteHeader(http.StatusBadRequest)
	})
	_, err := c.Create(context.Background(), CreateRequest{Version: 1, Nonce: make([]byte, 12), TTL: time.Hour})
	if !errors.Is(err, ErrRejected) || cl != 0 {
		t.Fatalf("err = %v, content-length = %d", err, cl)
	}
}

func TestCreateErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"too large", http.StatusRequestEntityTooLarge, `{"error":"size exceeded"}`, ErrTooLarge},
		{"server", http.StatusInternalServerError, `{"error":"internal"}`, ErrServer},
		{"bad json", http.StatusCreated, `{`, ErrProtocol},
		{"bad id", http.StatusCreated, `{"id":"x","expires_at":"2030-01-02T03:04:05Z","manage_token":"` + testManage.String() + `"}`, ErrProtocol},
		{"bad token", http.StatusCreated, `{"id":"` + testID.String() + `","expires_at":"2030-01-02T03:04:05Z","manage_token":"x"}`, ErrProtocol},
		{"no expiry", http.StatusCreated, `{"id":"` + testID.String() + `","manage_token":"` + testManage.String() + `"}`, ErrProtocol},
		{"huge body", http.StatusCreated, strings.Repeat(" ", maxJSONBody+1), ErrTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTLS(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.Create(context.Background(), CreateRequest{Version: 1, Nonce: make([]byte, 12), TTL: time.Hour, Body: []byte("x")})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "size exceeded") || strings.Contains(err.Error(), "internal") {
				t.Fatalf("server body leaked into error: %v", err)
			}
		})
	}
}

func TestCreateNetworkError(t *testing.T) {
	c, srv := newTLS(t, func(http.ResponseWriter, *http.Request) {})
	srv.Close()
	_, err := c.Create(context.Background(), CreateRequest{Version: 1, Nonce: make([]byte, 12), TTL: time.Hour, Body: []byte("x")})
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
}
