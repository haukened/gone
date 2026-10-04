package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

func TestHandleCreateSecretSuccess(t *testing.T) {
	h := httpx.New(createSecretSuccessService(t), 1024, nil)
	req := createSecretRequest(bytes.NewReader([]byte("cipher")))
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	assertCreateSecretSuccessResponse(t, w)
}

// createSecretSuccessService returns a service that validates create input and returns a fixed secret.
// It takes t for failures and returns a mock service for create success tests.
func createSecretSuccessService(t *testing.T) mockService {
	t.Helper()
	return mockService{createFn: func(_ context.Context, ct io.Reader, size int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
		b, err := io.ReadAll(ct)
		if err != nil {
			t.Fatalf("read create body: %v", err)
		}
		if string(b) != "cipher" {
			t.Fatalf("unexpected body")
		}
		if size != int64(len(b)) {
			t.Fatalf("size mismatch")
		}
		return app.Created{ID: domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ExpiresAt: time.Unix(1000, 0).UTC(), ManageToken: mustManage(t)}, nil
	}}
}

// createSecretRequest builds a valid create-secret request.
// It takes body as request content and returns an HTTP request with required headers.
func createSecretRequest(body *bytes.Reader) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/secret", body)
	req.Header.Set("Content-Length", "6")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
	req.Header.Set("X-Gone-TTL", "5m")
	return req
}

// assertCreateSecretSuccessResponse verifies the create-secret success HTTP response.
// It takes t for failures and w as the response to inspect.
func assertCreateSecretSuccessResponse(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %s", ct)
	}
	got := decodeCreateSecretResponse(t, w)
	if got.ID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || !got.ExpiresAt.Equal(time.Unix(1000, 0)) {
		t.Fatalf("unexpected body %+v", got)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control %q", cc)
	}
}

type createSecretResponse struct {
	ID          string    `json:"id"`
	ExpiresAt   time.Time `json:"expires_at"`
	ManageToken string    `json:"manage_token"`
}

// decodeCreateSecretResponse decodes and validates a create-secret response body.
// It takes t for failures and w as the response to decode, returning the parsed body.
func decodeCreateSecretResponse(t *testing.T, w *httptest.ResponseRecorder) createSecretResponse {
	t.Helper()
	var got createSecretResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := domain.ParseManageToken(got.ManageToken); err != nil {
		t.Fatalf("manage_token %q invalid: %v", got.ManageToken, err)
	}
	return got
}

func TestHandleCreateSecretValidationErrors(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/secret", bytes.NewReader([]byte("cipher")))
	// Intentionally omit Content-Length
	h := httpx.New(mockService{createFn: func(_ context.Context, _ io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
		return app.Created{}, nil
	}}, 10, nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusLengthRequired {
		t.Fatalf("expected 411 got %d", w.Code)
	}
}

func TestHandleClaimFreshSuccess(t *testing.T) {
	token, err := domain.NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	claimedUntil := time.Unix(2000, 0).UTC()
	h := httpx.New(freshClaimSuccessService(t, token, claimedUntil), 1024, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	assertFreshClaimResponse(t, w, token, claimedUntil)
}

// freshClaimSuccessService returns a service that validates a fresh claim and serves ciphertext.
// It takes t for failures, token for the expected response token, and claimedUntil for expiration.
func freshClaimSuccessService(t *testing.T, token domain.ClaimToken, claimedUntil time.Time) mockService {
	t.Helper()
	return mockService{claimFn: func(_ context.Context, id, gotToken string) (app.ClaimResult, error) {
		if id != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			t.Fatalf("id = %q", id)
		}
		if gotToken != "" {
			t.Fatalf("fresh claim token = %q, want empty", gotToken)
		}
		return app.ClaimResult{
			Claimed: app.Claimed{
				Meta:         app.Meta{Version: 1, NonceB64u: "n1"},
				Body:         io.NopCloser(bytes.NewReader([]byte("cipher"))),
				Size:         6,
				ClaimedUntil: claimedUntil,
			},
			Token: token,
		}, nil
	}}
}

// assertFreshClaimResponse verifies a fresh claim response and ciphertext body.
// It takes t for failures, w as the response, token, and claimedUntil.
func assertFreshClaimResponse(t *testing.T, w *httptest.ResponseRecorder, token domain.ClaimToken, claimedUntil time.Time) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	assertFreshClaimHeaders(t, w, token, claimedUntil)
	if !bytes.Equal(w.Body.Bytes(), []byte("cipher")) {
		t.Fatalf("body mismatch")
	}
}

// assertFreshClaimHeaders verifies metadata and cache headers on a fresh claim response.
// It takes t for failures, w as the response, token, and claimedUntil.
func assertFreshClaimHeaders(t *testing.T, w *httptest.ResponseRecorder, token domain.ClaimToken, claimedUntil time.Time) {
	t.Helper()
	assertFreshClaimMetadataHeaders(t, w, token, claimedUntil)
	assertFreshClaimBodyHeaders(t, w)
}

// assertFreshClaimMetadataHeaders verifies protocol, nonce, and claim headers.
// It takes t for failures, w as the response, token, and claimedUntil.
func assertFreshClaimMetadataHeaders(t *testing.T, w *httptest.ResponseRecorder, token domain.ClaimToken, claimedUntil time.Time) {
	t.Helper()
	if v := w.Header().Get("X-Gone-Version"); v != "1" {
		t.Fatalf("version header %s", v)
	}
	if n := w.Header().Get("X-Gone-Nonce"); n != "n1" {
		t.Fatalf("nonce header %s", n)
	}
	if got := w.Header().Get(httpx.HeaderClaim); got != token.String() {
		t.Fatalf("claim header = %q, want %q", got, token.String())
	}
	if got := w.Header().Get(httpx.HeaderClaimExpires); got != claimedUntil.Format(time.RFC3339) {
		t.Fatalf("claim expires = %q, want %q", got, claimedUntil.Format(time.RFC3339))
	}
}

// assertFreshClaimBodyHeaders verifies cache, content type, and content length headers.
// It takes t for failures and w as the response to inspect.
func assertFreshClaimBodyHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content-type = %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "6" {
		t.Fatalf("content-length = %q", got)
	}
}

func TestHandleClaimRetryPassesToken(t *testing.T) {
	token, err := domain.NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	called := false
	m := mockService{claimFn: func(_ context.Context, _ string, gotToken string) (app.ClaimResult, error) {
		called = true
		if gotToken != token.String() {
			t.Fatalf("retry token = %q, want %q", gotToken, token.String())
		}
		return app.ClaimResult{
			Claimed: app.Claimed{
				Meta:         app.Meta{Version: 1, NonceB64u: "n1"},
				Body:         io.NopCloser(bytes.NewReader([]byte("cipher"))),
				Size:         6,
				ClaimedUntil: time.Unix(2000, 0).UTC(),
			},
			Token: token,
		}, nil
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.Header.Set(httpx.HeaderClaim, token.String())
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if !called {
		t.Fatalf("expected Claim to be called")
	}
}
