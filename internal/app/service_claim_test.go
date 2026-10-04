package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/haukened/gone/internal/domain"
)

func TestServiceClaimInvalidID(t *testing.T) {
	ms := &mockStore{}
	svc := newServiceTestSubject(ms, time.Now())
	if _, err := svc.Claim(context.Background(), "not-an-id", ""); !errors.Is(err, domain.ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}
	if ms.claimCalled {
		t.Fatalf("store should not be called on invalid id")
	}
}

func TestServiceClaim(t *testing.T) {
	now := time.Unix(1700000500, 0).UTC()
	validID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	retryToken := mustClaimToken(t)
	storeErr := errors.New("claim failed")
	data := "ciphertext"

	tests := []serviceClaimCase{
		{name: "fresh claim issues token with default lease", wantLease: DefaultClaimLease, wantCalled: true},
		{name: "retry passes parsed token hash", token: retryToken.String(), wantRetry: true, wantLease: DefaultClaimLease, wantCalled: true},
		{name: "custom lease", lease: 90 * time.Second, wantLease: 90 * time.Second, wantCalled: true},
		{name: "invalid token", token: "bad-token", wantErr: domain.ErrInvalidClaim},
		{name: "store error", storeErr: storeErr, wantErr: storeErr, wantLease: DefaultClaimLease, wantCalled: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &mockStore{claimMeta: Meta{Version: 2, NonceB64u: "nonceX"}, claimData: data, claimSize: int64(len(data)), claimErr: tc.storeErr}
			svc := newServiceTestSubject(ms, now)
			svc.ClaimLease = tc.lease
			res, err := svc.Claim(context.Background(), validID, tc.token)
			assertClaimOutcome(t, tc, err, ms)
			if tc.wantErr == nil {
				assertClaimResult(t, res, claimAssertContext{
					store:      ms,
					tc:         tc,
					retryToken: retryToken,
					validID:    validID,
					data:       data,
					now:        now,
				})
			}
		})
	}
}

type serviceClaimCase struct {
	name       string
	token      string
	lease      time.Duration
	storeErr   error
	wantErr    error
	wantRetry  bool
	wantLease  time.Duration
	wantCalled bool
}

// assertClaimOutcome verifies claim errors and store call expectations.
func assertClaimOutcome(t *testing.T, tc serviceClaimCase, err error, ms *mockStore) {
	t.Helper()
	if !errors.Is(err, tc.wantErr) {
		t.Fatalf("Claim error = %v, want %v", err, tc.wantErr)
	}
	if ms.claimCalled != tc.wantCalled {
		t.Fatalf("claimCalled = %v, want %v", ms.claimCalled, tc.wantCalled)
	}
}

type claimAssertContext struct {
	store      *mockStore
	tc         serviceClaimCase
	retryToken domain.ClaimToken
	validID    string
	data       string
	now        time.Time
}

// assertClaimResult verifies a successful claim result and store inputs.
func assertClaimResult(t *testing.T, res ClaimResult, ctx claimAssertContext) {
	t.Helper()
	if res.Meta.Version != 2 || res.Meta.NonceB64u != "nonceX" {
		t.Fatalf("meta mismatch: %+v", res.Meta)
	}
	b, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if string(b) != ctx.data || res.Size != int64(len(ctx.data)) {
		t.Fatalf("claim data/size mismatch: %q/%d", string(b), res.Size)
	}
	assertClaimStoreInputs(t, res, ctx)
}

// assertClaimStoreInputs verifies claim persistence inputs and lease times.
func assertClaimStoreInputs(t *testing.T, res ClaimResult, ctx claimAssertContext) {
	t.Helper()
	if ctx.store.claimID != ctx.validID || ctx.store.claimRetry != ctx.tc.wantRetry {
		t.Fatalf("claim id/retry = %q/%v", ctx.store.claimID, ctx.store.claimRetry)
	}
	wantToken := expectedClaimToken(t, res.Token, ctx.tc.token, ctx.retryToken)
	if ctx.store.claimHash != wantToken.Hash() {
		t.Fatalf("claim hash = %q, want %q", ctx.store.claimHash, wantToken.Hash())
	}
	wantUntil := ctx.now.Add(ctx.tc.wantLease)
	if !ctx.store.claimClaimedUntil.Equal(wantUntil) || !res.ClaimedUntil.Equal(wantUntil) {
		t.Fatalf("claimedUntil store/result = %v/%v, want %v", ctx.store.claimClaimedUntil, res.ClaimedUntil, wantUntil)
	}
}

// expectedClaimToken returns the token expected for fresh and retry claims.
func expectedClaimToken(t *testing.T, got domain.ClaimToken, rawRetry string, retryToken domain.ClaimToken) domain.ClaimToken {
	t.Helper()
	if rawRetry == "" {
		if _, parseErr := domain.ParseClaimToken(got.String()); parseErr != nil {
			t.Fatalf("fresh token is invalid: %v", parseErr)
		}
		return got
	}
	if got.String() != rawRetry {
		t.Fatalf("retry token = %q, want %q", got.String(), rawRetry)
	}
	return retryToken
}

func TestServiceAck(t *testing.T) {
	validID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	validToken := mustClaimToken(t)
	storeErr := errors.New("ack failed")
	tests := []serviceAckCase{
		{name: "success increments consumed metric", id: validID, token: validToken.String(), wantCalled: true, wantMetric: 1},
		{name: "store error propagates without metric", id: validID, token: validToken.String(), storeErr: storeErr, wantErr: storeErr, wantCalled: true},
		{name: "invalid id", id: "bad-id", token: validToken.String(), wantErr: domain.ErrInvalidID},
		{name: "invalid token", id: validID, token: "bad-token", wantErr: domain.ErrInvalidClaim},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &mockStore{ackErr: tc.storeErr}
			metrics := &metricsRecorder{}
			svc := newServiceTestSubject(ms, time.Now())
			svc.Metrics = metrics
			err := svc.Ack(context.Background(), tc.id, tc.token)
			assertAckOutcome(t, tc, err, ms, metrics, validToken)
		})
	}
}

type serviceAckCase struct {
	name       string
	id         string
	token      string
	storeErr   error
	wantErr    error
	wantCalled bool
	wantMetric int64
}

// assertAckOutcome verifies Ack errors, store inputs, and metrics.
func assertAckOutcome(t *testing.T, tc serviceAckCase, err error, ms *mockStore, metrics *metricsRecorder, validToken domain.ClaimToken) {
	t.Helper()
	if !errors.Is(err, tc.wantErr) {
		t.Fatalf("Ack error = %v, want %v", err, tc.wantErr)
	}
	if ms.ackCalled != tc.wantCalled {
		t.Fatalf("ackCalled = %v, want %v", ms.ackCalled, tc.wantCalled)
	}
	if tc.wantCalled && (ms.ackID != tc.id || ms.ackHash != validToken.Hash()) {
		t.Fatalf("ack args = %q/%q", ms.ackID, ms.ackHash)
	}
	if got := metrics.count("secrets_consumed_total"); got != tc.wantMetric {
		t.Fatalf("consumed metric = %d, want %d", got, tc.wantMetric)
	}
}
