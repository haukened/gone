package httpx_test

import (
	"context"
	"io"
	"time"

	"github.com/haukened/gone/internal/app"
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
