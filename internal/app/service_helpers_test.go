package app

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// fixedClock implements Clock by returning one fixed instant.
type fixedClock struct{ now time.Time }

// Now returns the fixed instant configured on the clock.
//
// Returns:
//   - time.Time: the fixed test instant.
func (f fixedClock) Now() time.Time { return f.now }

// mockStore implements SecretStore for service tests.
type mockStore struct {
	saveErr error

	claimMeta Meta
	claimData string
	claimSize int64
	claimErr  error

	ackErr error

	status    SecretStatus
	statusErr error
	revokeErr error

	savedManageHash string
	savedID         string
	savedMeta       Meta
	savedSize       int64
	savedExpires    time.Time
	saveCalled      bool

	claimCalled       bool
	claimID           string
	claimHash         string
	claimRetry        bool
	claimClaimedUntil time.Time

	ackCalled bool
	ackID     string
	ackHash   string

	statusID   string
	statusHash string

	revokeCalled bool
	revokeID     string
	revokeHash   string
}

// Save records persisted secret inputs and returns the configured error.
func (m *mockStore) Save(ctx context.Context, id string, meta Meta, manageHash string, r io.Reader, size int64, expiresAt time.Time) error {
	_ = ctx
	_ = r
	m.saveCalled = true
	m.savedManageHash = manageHash
	m.savedID = id
	m.savedMeta = meta
	m.savedSize = size
	m.savedExpires = expiresAt
	return m.saveErr
}

// Claim records claim inputs and returns the configured claimed secret.
func (m *mockStore) Claim(ctx context.Context, id, claimHash string, retry bool, claimedUntil time.Time) (Claimed, error) {
	_ = ctx
	m.claimCalled = true
	m.claimID = id
	m.claimHash = claimHash
	m.claimRetry = retry
	m.claimClaimedUntil = claimedUntil
	if m.claimErr != nil {
		return Claimed{}, m.claimErr
	}
	return Claimed{
		Meta:         m.claimMeta,
		Body:         io.NopCloser(strings.NewReader(m.claimData)),
		Size:         m.claimSize,
		ClaimedUntil: claimedUntil,
	}, nil
}

// Status records status inputs and returns the configured status.
func (m *mockStore) Status(_ context.Context, id, manageHash string) (SecretStatus, error) {
	m.statusID = id
	m.statusHash = manageHash
	return m.status, m.statusErr
}

// Revoke records revoke inputs and returns the configured error.
func (m *mockStore) Revoke(_ context.Context, id, manageHash string) error {
	m.revokeCalled = true
	m.revokeID = id
	m.revokeHash = manageHash
	return m.revokeErr
}

// Ack records acknowledge inputs and returns the configured error.
func (m *mockStore) Ack(ctx context.Context, id, claimHash string) error {
	_ = ctx
	m.ackCalled = true
	m.ackID = id
	m.ackHash = claimHash
	return m.ackErr
}

// DeleteExpired satisfies SecretStore without affecting service tests.
func (m *mockStore) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	_ = ctx
	_ = t
	return 0, nil
}

// Reconcile satisfies SecretStore without affecting service tests.
func (m *mockStore) Reconcile(ctx context.Context) error {
	_ = ctx
	return nil
}

// metricsRecorder captures service metric increments for assertions.
type metricsRecorder struct {
	counts map[string]int64
}

// Inc records the named metric delta.
func (m *metricsRecorder) Inc(name string, delta int64) {
	if m.counts == nil {
		m.counts = make(map[string]int64)
	}
	m.counts[name] += delta
}

// count returns the recorded total for name.
func (m *metricsRecorder) count(name string) int64 {
	if m.counts == nil {
		return 0
	}
	return m.counts[name]
}

// mustClaimToken returns a valid claim token for tests.
func mustClaimToken(t *testing.T) domain.ClaimToken {
	t.Helper()
	tok, err := domain.NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	return tok
}

// mustManageToken returns a valid manage token for tests.
func mustManageToken(t *testing.T) domain.ManageToken {
	t.Helper()
	tok, err := domain.NewManageToken()
	if err != nil {
		t.Fatalf("NewManageToken: %v", err)
	}
	return tok
}

// newServiceTestSubject returns a service with standard test limits.
func newServiceTestSubject(store SecretStore, now time.Time) *Service {
	return &Service{
		Store:      store,
		Clock:      fixedClock{now: now},
		MaxBytes:   100,
		MinTTL:     time.Minute,
		MaxTTL:     5 * time.Minute,
		ClaimLease: DefaultClaimLease,
	}
}
