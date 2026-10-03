package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

type mockService struct {
	createFn func(ctx context.Context, ct io.Reader, size int64, _ uint8, _ string, _ time.Duration) (app.Created, error)
	claimFn  func(ctx context.Context, id, token string) (app.ClaimResult, error)
	ackFn    func(ctx context.Context, id, token string) error
	statusFn func(ctx context.Context, id, token string) (app.SecretStatus, error)
	revokeFn func(ctx context.Context, id, token string) error
}

func (m mockService) CreateSecret(ctx context.Context, ct io.Reader, size int64, version uint8, nonce string, ttl time.Duration) (app.Created, error) {
	if m.createFn == nil {
		return app.Created{}, nil
	}
	return m.createFn(ctx, ct, size, version, nonce, ttl)
}

func (m mockService) Status(ctx context.Context, idStr, token string) (app.SecretStatus, error) {
	if m.statusFn == nil {
		return app.SecretStatus{}, app.ErrNotFound
	}
	return m.statusFn(ctx, idStr, token)
}

func (m mockService) Revoke(ctx context.Context, idStr, token string) error {
	if m.revokeFn == nil {
		return app.ErrNotFound
	}
	return m.revokeFn(ctx, idStr, token)
}

func (m mockService) Claim(ctx context.Context, idStr, token string) (app.ClaimResult, error) {
	if m.claimFn == nil {
		return app.ClaimResult{}, nil
	}
	return m.claimFn(ctx, idStr, token)
}

func (m mockService) Ack(ctx context.Context, idStr, token string) error {
	if m.ackFn == nil {
		return nil
	}
	return m.ackFn(ctx, idStr, token)
}

func TestHandleCreateSecretSuccess(t *testing.T) {
	m := mockService{createFn: func(_ context.Context, ct io.Reader, size int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
		b, _ := io.ReadAll(ct)
		if string(b) != "cipher" {
			t.Fatalf("unexpected body")
		}
		if size != int64(len(b)) {
			t.Fatalf("size mismatch")
		}
		return app.Created{ID: domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ExpiresAt: time.Unix(1000, 0).UTC(), ManageToken: mustManage(t)}, nil
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/secret", bytes.NewReader([]byte("cipher")))
	req.Header.Set("Content-Length", "6")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
	req.Header.Set("X-Gone-TTL", "5m")
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %s", ct)
	}
	var got struct {
		ID          string    `json:"id"`
		ExpiresAt   time.Time `json:"expires_at"`
		ManageToken string    `json:"manage_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || !got.ExpiresAt.Equal(time.Unix(1000, 0)) {
		t.Fatalf("unexpected body %+v", got)
	}
	if _, err := domain.ParseManageToken(got.ManageToken); err != nil {
		t.Fatalf("manage_token %q invalid: %v", got.ManageToken, err)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control %q", cc)
	}
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
	m := mockService{claimFn: func(_ context.Context, id, gotToken string) (app.ClaimResult, error) {
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
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
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
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content-type = %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "6" {
		t.Fatalf("content-length = %q", got)
	}
	if !bytes.Equal(w.Body.Bytes(), []byte("cipher")) {
		t.Fatalf("body mismatch")
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

func TestHandleAckDeleteRouted(t *testing.T) {
	token, err := domain.NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	called := false
	m := mockService{ackFn: func(_ context.Context, id, gotToken string) error {
		called = true
		if id != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			t.Fatalf("id = %q", id)
		}
		if gotToken != token.String() {
			t.Fatalf("ack token = %q, want %q", gotToken, token.String())
		}
		return nil
	}}
	h := httpx.New(m, 1024, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.Header.Set(httpx.HeaderClaim, token.String())
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
	if !called {
		t.Fatalf("expected Ack to be called")
	}
}

func TestHealthAndReady(t *testing.T) {
	readyCalled := false
	readiness := func(context.Context) error { readyCalled = true; return nil }
	h := httpx.New(mockService{}, 10, readiness)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health status %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ready status %d", w.Code)
	}
	if !readyCalled {
		t.Fatalf("readiness not invoked")
	}
}
func TestHandleSecretPage(t *testing.T) {
	// provide a minimal secret template
	tmpl := template.Must(template.New("secret").Parse(`<!DOCTYPE html><html><body>{{template "header" .}}<div id="view-open"></div></body></html>`))
	h := httpx.New(mockService{}, 1024, nil)
	// Need partials header template to satisfy reference; keep it simple
	tmplWithPartials := template.Must(template.New("partials").Parse(`{{define "header"}}<header>H</header>{{end}}`))
	tmplWithPartials, _ = tmplWithPartials.AddParseTree("secret", tmpl.Tree)
	h.SecretTmpl = httpx.TemplateRenderer{T: tmplWithPartials}
	req := httptest.NewRequest(http.MethodGet, "/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type %s", ct)
	}
}
