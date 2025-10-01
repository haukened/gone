package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/domain"
)

// fixedClock implements Clock returning a fixed instant.
type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

// mockStore implements SecretStore for tests.
type mockStore struct {
	saveErr     error
	consumeMeta Meta
	consumeData string
	consumeSize int64
	consumeErr  error

	// captured on Save
	savedID      string
	savedMeta    Meta
	savedSize    int64
	savedExpires time.Time
	saveCalled   bool

	consumeCalled bool
}

func (m *mockStore) Save(ctx context.Context, id string, meta Meta, r io.Reader, size int64, expiresAt time.Time) error {
	_ = ctx
	_ = r
	m.saveCalled = true
	m.savedID = id
	m.savedMeta = meta
	m.savedSize = size
	m.savedExpires = expiresAt
	return m.saveErr
}

func (m *mockStore) Consume(ctx context.Context, id string) (Meta, io.ReadCloser, int64, error) {
	_ = ctx
	_ = id
	m.consumeCalled = true
	if m.consumeErr != nil {
		return Meta{}, nil, 0, m.consumeErr
	}
	return m.consumeMeta, io.NopCloser(strings.NewReader(m.consumeData)), m.consumeSize, nil
}

func (m *mockStore) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	_ = ctx
	_ = t
	return 0, nil
}
func (m *mockStore) Reconcile(ctx context.Context) error { _ = ctx; return nil }

func TestServiceCreateSecretSuccess(t *testing.T) {
	ms := &mockStore{}
	now := time.Unix(1700000000, 0)
	svc := &Service{Store: ms, Clock: fixedClock{now: now}, MaxBytes: 1024, MinTTL: time.Minute, MaxTTL: 10 * time.Minute}
	data := "ciphertext"
	ttl := 2 * time.Minute
	id, exp, err := svc.CreateSecret(context.Background(), strings.NewReader(data), int64(len(data)), 1, "nonce123", ttl)
	if err != nil {
		t.Fatalf("CreateSecret error: %v", err)
	}
	if !id.Valid() {
		t.Fatalf("returned id invalid: %s", id)
	}
	if exp != now.Add(ttl) {
		t.Fatalf("expiry mismatch: got %v want %v", exp, now.Add(ttl))
	}
	if !ms.saveCalled {
		t.Fatalf("expected Save to be called")
	}
	if ms.savedID != id.String() {
		t.Fatalf("savedID mismatch")
	}
	if ms.savedMeta.Version != 1 || ms.savedMeta.NonceB64u != "nonce123" {
		t.Fatalf("meta mismatch: %+v", ms.savedMeta)
	}
	if ms.savedSize != int64(len(data)) {
		t.Fatalf("size mismatch: %d", ms.savedSize)
	}
	if ms.savedExpires != exp {
		t.Fatalf("expires mismatch: %v vs %v", ms.savedExpires, exp)
	}
}

func TestServiceCreateSecretTTLInvalid(t *testing.T) {
	ms := &mockStore{}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 1024, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	// below min
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader("a"), 1, 1, "n", 30*time.Second); err != domain.ErrTTLInvalid {
		t.Fatalf("expected ErrTTLInvalid for below min, got %v", err)
	}
	// above max
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader("a"), 1, 1, "n", 10*time.Minute); err != domain.ErrTTLInvalid {
		t.Fatalf("expected ErrTTLInvalid for above max, got %v", err)
	}
}

func TestServiceCreateSecretSizeValidation(t *testing.T) {
	ms := &mockStore{}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 10, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader(""), 0, 1, "n", time.Minute); err != ErrSizeExceeded {
		t.Fatalf("expected ErrSizeExceeded for size 0, got %v", err)
	}
	if _, _, err := svc.CreateSecret(context.Background(), strings.NewReader("01234567890"), 11, 1, "n", time.Minute); err != ErrSizeExceeded {
		t.Fatalf("expected ErrSizeExceeded for oversize, got %v", err)
	}
}

func TestServiceCreateSecretStoreError(t *testing.T) {
	boom := errors.New("boom")
	ms := &mockStore{saveErr: boom}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 100, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	_, _, err := svc.CreateSecret(context.Background(), strings.NewReader("abc"), 3, 1, "n", 2*time.Minute)
	if err != boom {
		t.Fatalf("expected store error propagation, got %v", err)
	}
	if !ms.saveCalled {
		t.Fatalf("expected save called")
	}
}

func TestServiceConsumeInvalidID(t *testing.T) {
	ms := &mockStore{}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 100, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	if _, _, _, err := svc.Consume(context.Background(), "not-an-id"); err != domain.ErrInvalidID {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}
	if ms.consumeCalled {
		t.Fatalf("store should not be called on invalid id")
	}
}

func TestServiceConsumeSuccess(t *testing.T) {
	data := "ciphertext"
	ms := &mockStore{consumeMeta: Meta{Version: 2, NonceB64u: "nonceX"}, consumeData: data, consumeSize: int64(len(data))}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 100, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	id, _ := domain.NewID()
	meta, rc, size, err := svc.Consume(context.Background(), id.String())
	if err != nil {
		t.Fatalf("Consume error: %v", err)
	}
	if meta.Version != 2 || meta.NonceB64u != "nonceX" {
		t.Fatalf("meta mismatch: %+v", meta)
	}
	b, _ := io.ReadAll(rc)
	if string(b) != data {
		t.Fatalf("data mismatch: %s", string(b))
	}
	if size != int64(len(data)) {
		t.Fatalf("size mismatch: %d", size)
	}
	if !ms.consumeCalled {
		t.Fatalf("expected consume called")
	}
}

func TestServiceConsumeStoreError(t *testing.T) {
	sentinel := errors.New("notfound")
	ms := &mockStore{consumeErr: sentinel}
	svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, MaxBytes: 100, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
	id, _ := domain.NewID()
	_, _, _, err := svc.Consume(context.Background(), id.String())
	if err != sentinel {
		t.Fatalf("expected store consume error, got %v", err)
	}
}
