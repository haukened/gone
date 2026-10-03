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

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

type consumeService struct { // reuse custom service for consume errors
	claimErr error
	ackErr   error
}

func (c consumeService) CreateSecret(_ context.Context, _ io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
	return app.Created{ID: domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (consumeService) Status(context.Context, string, string) (app.SecretStatus, error) {
	return app.SecretStatus{}, app.ErrNotFound
}

func (consumeService) Revoke(context.Context, string, string) error { return app.ErrNotFound }

func (c consumeService) Claim(_ context.Context, _ string, _ string) (app.ClaimResult, error) {
	if c.claimErr != nil {
		return app.ClaimResult{}, c.claimErr
	}
	return app.ClaimResult{
		Claimed: app.Claimed{
			Meta:         app.Meta{Version: 1, NonceB64u: "n"},
			Body:         io.NopCloser(bytes.NewReader([]byte("ok"))),
			Size:         2,
			ClaimedUntil: time.Unix(2000, 0).UTC(),
		},
		Token: domain.ClaimToken("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	}, nil
}

func (c consumeService) Ack(_ context.Context, _ string, _ string) error {
	if c.ackErr != nil {
		return c.ackErr
	}
	return nil
}

func TestConsumeEndpointErrors(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		service        httpx.ServicePort
		expectCode     int
		expectContains string
		expectAllow    string
	}{
		{name: "method not allowed", method: http.MethodPost, path: "/api/secret/abcd", expectCode: http.StatusMethodNotAllowed, expectContains: "method not allowed", expectAllow: "GET, DELETE"},
		// GET /api/secret hits the create handler path and fails method guard -> 405
		{name: "get without id -> 405", method: http.MethodGet, path: "/api/secret", expectCode: http.StatusMethodNotAllowed, expectContains: "method not allowed"},
		// GET /api/secret/ matches consume handler but missing id -> 404 not found
		{name: "missing id -> 404", method: http.MethodGet, path: "/api/secret/", expectCode: http.StatusNotFound, expectContains: "not found"},
		{name: "claim not found", method: http.MethodGet, path: "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", service: consumeService{claimErr: app.ErrNotFound}, expectCode: http.StatusNotFound, expectContains: "not found"},
		{name: "claim invalid id", method: http.MethodGet, path: "/api/secret/bad-id-!!!", service: consumeService{claimErr: domain.ErrInvalidID}, expectCode: http.StatusBadRequest, expectContains: "invalid id"},
		{name: "claim invalid token", method: http.MethodGet, path: "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", service: consumeService{claimErr: domain.ErrInvalidClaim}, expectCode: http.StatusBadRequest, expectContains: "invalid claim"},
		{name: "claim internal error", method: http.MethodGet, path: "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", service: consumeService{claimErr: errors.New("boom")}, expectCode: http.StatusInternalServerError, expectContains: "internal"},
		{name: "delete not found", method: http.MethodDelete, path: "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", service: consumeService{ackErr: app.ErrNotFound}, expectCode: http.StatusNotFound, expectContains: "not found"},
		{name: "delete invalid claim", method: http.MethodDelete, path: "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", service: consumeService{ackErr: domain.ErrInvalidClaim}, expectCode: http.StatusBadRequest, expectContains: "invalid claim"},
		{name: "delete invalid id", method: http.MethodDelete, path: "/api/secret/bad-id-!!!", service: consumeService{ackErr: domain.ErrInvalidID}, expectCode: http.StatusBadRequest, expectContains: "invalid id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := tc.service
			if svc == nil {
				svc = consumeService{}
			}
			h := httpx.New(svc, 1024, nil)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, req)
			if w.Code != tc.expectCode {
				t.Fatalf("expected status %d got %d body=%s", tc.expectCode, w.Code, w.Body.String())
			}
			if !bytes.Contains(w.Body.Bytes(), []byte(tc.expectContains)) {
				t.Fatalf("expected body to contain %q got %s", tc.expectContains, w.Body.String())
			}
			if tc.expectAllow != "" && w.Header().Get("Allow") != tc.expectAllow {
				t.Fatalf("Allow = %q, want %q", w.Header().Get("Allow"), tc.expectAllow)
			}
		})
	}
}
