package httpx_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/httpx"
)

type failingService struct { // implements ServicePort for error injection
	fail bool
}

func (f failingService) CreateSecret(_ context.Context, _ io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
	if f.fail {
		return app.Created{}, errors.New("boom")
	}
	return app.Created{ID: domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (failingService) Status(context.Context, string, string) (app.SecretStatus, error) {
	return app.SecretStatus{}, app.ErrNotFound
}

func (failingService) Revoke(context.Context, string, string) error { return app.ErrNotFound }

func (f failingService) Claim(_ context.Context, _ string, _ string) (app.ClaimResult, error) {
	return app.ClaimResult{}, errors.New("unused")
}

func (f failingService) Ack(_ context.Context, _ string, _ string) error {
	return errors.New("unused")
}

func TestCreateEndpointErrors(t *testing.T) {
	commonHeaders := func(h http.Header) {
		h.Set("Content-Length", "10")
		h.Set("X-Gone-Version", "1")
		h.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
		h.Set("X-Gone-TTL", "5m")
	}
	tests := []struct {
		name               string
		method             string
		path               string
		mutateReq          func(*http.Request)
		service            httpx.ServicePort
		expectCode         int
		expectBodyContains string
	}{
		{name: "method not allowed", method: http.MethodGet, path: "/api/secret", expectCode: http.StatusMethodNotAllowed, expectBodyContains: "method not allowed"},
		// A POST to /api/secret/extra is routed to the consume handler path prefix (/api/secret/) which expects GET, so we receive 405.
		{name: "path mismatch", method: http.MethodPost, path: "/api/secret/extra", expectCode: http.StatusMethodNotAllowed, expectBodyContains: "method not allowed"},
		{name: "missing content-length", method: http.MethodPost, path: "/api/secret", mutateReq: func(r *http.Request) { r.Header.Del("Content-Length") }, expectCode: http.StatusLengthRequired, expectBodyContains: "content length required"},
		{name: "invalid content-length", method: http.MethodPost, path: "/api/secret", mutateReq: func(r *http.Request) { r.Header.Set("Content-Length", "-5") }, expectCode: http.StatusBadRequest, expectBodyContains: "invalid content length"},
		{name: "size exceeded", method: http.MethodPost, path: "/api/secret", mutateReq: func(r *http.Request) { r.Header.Set("Content-Length", "999999999") }, expectCode: http.StatusRequestEntityTooLarge, expectBodyContains: "size exceeded"},
		{name: "missing required headers", method: http.MethodPost, path: "/api/secret", mutateReq: func(r *http.Request) { r.Header.Del("X-Gone-Version") }, expectCode: http.StatusBadRequest, expectBodyContains: "missing required headers"},
		{name: "invalid version", method: http.MethodPost, path: "/api/secret", mutateReq: func(r *http.Request) { r.Header.Set("X-Gone-Version", "9999") }, expectCode: http.StatusBadRequest, expectBodyContains: "invalid version"},
		{name: "invalid ttl", method: http.MethodPost, path: "/api/secret", mutateReq: func(r *http.Request) { r.Header.Set("X-Gone-TTL", "zzz") }, expectCode: http.StatusBadRequest, expectBodyContains: "invalid ttl"},
		{name: "service internal error mapping", method: http.MethodPost, path: "/api/secret", service: failingService{fail: true}, expectCode: http.StatusInternalServerError, expectBodyContains: "internal"},
	}

	for _, tc := range tests {
		// using subtest for isolation
		t.Run(tc.name, func(t *testing.T) {
			body := bytes.NewReader(make([]byte, 0))
			req := httptest.NewRequest(tc.method, tc.path, body)
			commonHeaders(req.Header)
			if tc.mutateReq != nil {
				tc.mutateReq(req)
			}
			svc := tc.service
			if svc == nil {
				svc = failingService{fail: false}
			}
			h := httpx.New(svc, 1024, nil)
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, req)
			if w.Code != tc.expectCode {
				t.Fatalf("expected status %d got %d body=%s", tc.expectCode, w.Code, w.Body.String())
			}
			if !bytes.Contains(w.Body.Bytes(), []byte(tc.expectBodyContains)) {
				t.Fatalf("expected body to contain %q got %s", tc.expectBodyContains, w.Body.String())
			}
		})
	}
}
