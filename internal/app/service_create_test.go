package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

func TestServiceCreateSecretSuccess(t *testing.T) {
	ms := &mockStore{}
	now := time.Unix(1700000000, 0)
	svc := &Service{Store: ms, Clock: fixedClock{now: now}, MaxBytes: 1024, MinTTL: time.Minute, MaxTTL: 10 * time.Minute}
	data := "ciphertext"
	ttl := 2 * time.Minute

	created, err := svc.CreateSecret(context.Background(), strings.NewReader(data), int64(len(data)), 1, "AAAAAAAAAAAAAAAA", ttl)
	if err != nil {
		t.Fatalf("CreateSecret error: %v", err)
	}
	assertCreatedSecret(t, ms, created, now.Add(ttl), int64(len(data)))
}

// assertCreatedSecret verifies successful CreateSecret outputs and store inputs.
func assertCreatedSecret(t *testing.T, ms *mockStore, created Created, wantExpires time.Time, wantSize int64) {
	t.Helper()
	if _, err := domain.ParseManageToken(created.ManageToken.String()); err != nil {
		t.Fatalf("manage token invalid: %v", err)
	}
	if ms.savedManageHash != created.ManageToken.Hash() {
		t.Fatalf("saved manage hash mismatch")
	}
	if !created.ID.Valid() {
		t.Fatalf("returned id invalid: %s", created.ID)
	}
	if created.ExpiresAt != wantExpires {
		t.Fatalf("expiry mismatch: got %v want %v", created.ExpiresAt, wantExpires)
	}
	assertSaveCall(t, ms, created, wantSize)
}

// assertSaveCall verifies the mock store captured the expected save request.
func assertSaveCall(t *testing.T, ms *mockStore, created Created, wantSize int64) {
	t.Helper()
	if !ms.saveCalled {
		t.Fatalf("expected Save to be called")
	}
	if ms.savedID != created.ID.String() {
		t.Fatalf("savedID mismatch")
	}
	if ms.savedMeta.Version != 1 || ms.savedMeta.NonceB64u != "AAAAAAAAAAAAAAAA" {
		t.Fatalf("meta mismatch: %+v", ms.savedMeta)
	}
	if ms.savedSize != wantSize {
		t.Fatalf("size mismatch: %d", ms.savedSize)
	}
	if ms.savedExpires != created.ExpiresAt {
		t.Fatalf("expires mismatch: %v vs %v", ms.savedExpires, created.ExpiresAt)
	}
}

func TestServiceCreateSecretTTLInvalid(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{name: "below min", ttl: 30 * time.Second},
		{name: "above max", ttl: 10 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newServiceTestSubject(&mockStore{}, time.Now())
			_, err := svc.CreateSecret(context.Background(), strings.NewReader("a"), 1, 1, "AAAAAAAAAAAAAAAA", tc.ttl)
			if !errors.Is(err, domain.ErrTTLInvalid) {
				t.Fatalf("expected ErrTTLInvalid, got %v", err)
			}
		})
	}
}

func TestServiceCreateSecretProtocolValidation(t *testing.T) {
	cases := []struct {
		name    string
		version uint8
		nonce   string
		want    error
	}{
		{"unsupported version", 3, "AAAAAAAAAAAAAAAA", domain.ErrInvalidVersion},
		{"zero version", 0, "AAAAAAAAAAAAAAAA", domain.ErrInvalidVersion},
		{"short nonce", 1, "n", domain.ErrInvalidNonce},
		{"non-canonical nonce", 1, "AAAAAAAAAAAAAAA=", domain.ErrInvalidNonce},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms := &mockStore{}
			svc := newServiceTestSubject(ms, time.Now())
			_, err := svc.CreateSecret(context.Background(), strings.NewReader("a"), 1, c.version, c.nonce, time.Minute)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if ms.saveCalled {
				t.Fatalf("store must not be called")
			}
		})
	}
}

func TestServiceCreateSecretSizeValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
		size int64
	}{
		{name: "empty", data: "", size: 0},
		{name: "oversize", data: "01234567890", size: 11},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{Store: &mockStore{}, Clock: fixedClock{now: time.Now()}, MaxBytes: 10, MinTTL: time.Minute, MaxTTL: 5 * time.Minute}
			_, err := svc.CreateSecret(context.Background(), strings.NewReader(tc.data), tc.size, 1, "AAAAAAAAAAAAAAAA", time.Minute)
			if !errors.Is(err, ErrSizeExceeded) {
				t.Fatalf("expected ErrSizeExceeded, got %v", err)
			}
		})
	}
}

func TestServiceCreateSecretStoreError(t *testing.T) {
	boom := errors.New("boom")
	ms := &mockStore{saveErr: boom}
	svc := newServiceTestSubject(ms, time.Now())
	_, err := svc.CreateSecret(context.Background(), strings.NewReader("abc"), 3, 1, "AAAAAAAAAAAAAAAA", 2*time.Minute)
	if !errors.Is(err, boom) {
		t.Fatalf("expected store error propagation, got %v", err)
	}
	if !ms.saveCalled {
		t.Fatalf("expected save called")
	}
}
