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
	invalid  bool
	internal bool
}

func (c consumeService) CreateSecret(_ context.Context, _ io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (domain.SecretID, time.Time, error) {
	return domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), time.Now().Add(time.Hour), nil
}
func (c consumeService) Consume(_ context.Context, id string) (app.Meta, io.ReadCloser, int64, error) {
	if c.invalid {
		return app.Meta{}, nil, 0, domain.ErrInvalidID
	}
	if c.internal {
		return app.Meta{}, nil, 0, errors.New("boom")
	}
	return app.Meta{Version: 1, NonceB64u: "n"}, io.NopCloser(bytes.NewReader([]byte("ok"))), 2, nil
}

func TestConsumeEndpointErrors(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		service        httpx.ServicePort
		expectCode     int
		expectContains string
	}{
		{name: "method not allowed", method: http.MethodPost, path: "/api/secret/abcd", expectCode: http.StatusMethodNotAllowed, expectContains: "method not allowed"},
		// GET /api/secret hits the create handler path and fails method guard -> 405
		{name: "get without id -> 405", method: http.MethodGet, path: "/api/secret", expectCode: http.StatusMethodNotAllowed, expectContains: "method not allowed"},
		// GET /api/secret/ matches consume handler but missing id -> 404 not found
		{name: "missing id -> 404", method: http.MethodGet, path: "/api/secret/", expectCode: http.StatusNotFound, expectContains: "not found"},
		{name: "invalid id", method: http.MethodGet, path: "/api/secret/bad-id-!!!", service: consumeService{invalid: true}, expectCode: http.StatusBadRequest, expectContains: "invalid id"},
		{name: "internal error", method: http.MethodGet, path: "/api/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", service: consumeService{internal: true}, expectCode: http.StatusInternalServerError, expectContains: "internal"},
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
		})
	}
}
