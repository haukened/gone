package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

func TestServiceStatus(t *testing.T) {
	validID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tok := mustManageToken(t)
	now := time.Unix(1700000000, 0)
	want := SecretStatus{CreatedAt: now, ExpiresAt: now.Add(time.Hour)}

	tests := []struct {
		name     string
		id       string
		token    string
		storeErr error
		wantErr  error
		wantHash string
	}{
		{name: "success", id: validID, token: tok.String(), wantHash: tok.Hash()},
		{name: "not found propagates", id: validID, token: tok.String(), storeErr: ErrNotFound, wantErr: ErrNotFound, wantHash: tok.Hash()},
		{name: "invalid id", id: "bad-id", token: tok.String(), wantErr: domain.ErrInvalidID},
		{name: "invalid token", id: validID, token: "bad", wantErr: domain.ErrInvalidManage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &mockStore{status: want, statusErr: tc.storeErr}
			svc := &Service{Store: ms, Clock: fixedClock{now: now}}
			got, err := svc.Status(context.Background(), tc.id, tc.token)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Status error = %v, want %v", err, tc.wantErr)
			}
			if ms.statusHash != tc.wantHash {
				t.Fatalf("status hash = %q, want %q", ms.statusHash, tc.wantHash)
			}
			if tc.wantErr == nil && got != want {
				t.Fatalf("Status = %+v, want %+v", got, want)
			}
		})
	}
}

func TestServiceRevoke(t *testing.T) {
	validID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tok := mustManageToken(t)
	storeErr := errors.New("revoke failed")
	tests := []serviceRevokeCase{
		{name: "success increments revoked metric", id: validID, token: tok.String(), wantCalled: true, wantMetric: 1},
		{name: "store error propagates without metric", id: validID, token: tok.String(), storeErr: storeErr, wantErr: storeErr, wantCalled: true},
		{name: "not found propagates", id: validID, token: tok.String(), storeErr: ErrNotFound, wantErr: ErrNotFound, wantCalled: true},
		{name: "invalid id", id: "bad-id", token: tok.String(), wantErr: domain.ErrInvalidID},
		{name: "invalid token", id: validID, token: "bad", wantErr: domain.ErrInvalidManage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &mockStore{revokeErr: tc.storeErr}
			metrics := &metricsRecorder{}
			svc := &Service{Store: ms, Clock: fixedClock{now: time.Now()}, Metrics: metrics}
			err := svc.Revoke(context.Background(), tc.id, tc.token)
			assertRevokeOutcome(t, tc, err, ms, metrics, tok)
		})
	}
}

type serviceRevokeCase struct {
	name       string
	id         string
	token      string
	storeErr   error
	wantErr    error
	wantCalled bool
	wantMetric int64
}

// assertRevokeOutcome verifies Revoke errors, store inputs, and metrics.
func assertRevokeOutcome(t *testing.T, tc serviceRevokeCase, err error, ms *mockStore, metrics *metricsRecorder, tok domain.ManageToken) {
	t.Helper()
	if !errors.Is(err, tc.wantErr) {
		t.Fatalf("Revoke error = %v, want %v", err, tc.wantErr)
	}
	if ms.revokeCalled != tc.wantCalled {
		t.Fatalf("revokeCalled = %v, want %v", ms.revokeCalled, tc.wantCalled)
	}
	if tc.wantCalled && (ms.revokeID != tc.id || ms.revokeHash != tok.Hash()) {
		t.Fatalf("revoke args = %q/%q", ms.revokeID, ms.revokeHash)
	}
	if got := metrics.count("secrets_revoked_total"); got != tc.wantMetric {
		t.Fatalf("revoked metric = %d, want %d", got, tc.wantMetric)
	}
}

func TestServiceRevokeNilMetrics(t *testing.T) {
	svc := &Service{Store: &mockStore{}, Clock: fixedClock{now: time.Now()}}
	err := svc.Revoke(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", mustManageToken(t).String())
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
}
