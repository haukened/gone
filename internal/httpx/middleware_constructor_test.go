package httpx_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

type ctorService struct{}

func (ctorService) CreateSecret(context.Context, io.Reader, int64, uint8, string, time.Duration) (domain.SecretID, time.Time, error) {
	return domain.SecretID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), time.Now(), nil
}
func (ctorService) Claim(context.Context, string, string) (app.ClaimResult, error) {
	return app.ClaimResult{Claimed: app.Claimed{Body: io.NopCloser(nil)}}, nil
}

func (ctorService) Ack(context.Context, string, string) error {
	return nil
}

func TestHandlerConstructor(t *testing.T) {
	rd := func(context.Context) error { return nil }
	h := httpx.New(ctorService{}, 4096, rd)
	if h.Service == nil {
		t.Fatalf("expected service set")
	}
	if h.MaxBody != 4096 {
		t.Fatalf("expected maxBody 4096 got %d", h.MaxBody)
	}
	if h.Readiness == nil {
		t.Fatalf("expected readiness set")
	}
}
