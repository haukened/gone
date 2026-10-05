package store_test

import (
	"context"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

// Status returns the configured status or error.
//
// Parameters:
//   - ctx: ignored context.
//   - id: ignored secret id.
//   - manageHash: ignored manage hash.
//   - now: ignored current time.
//
// Returns: the configured status and status error.
func (m storeMockIndex) Status(_ context.Context, _, _ string, _ time.Time) (app.SecretStatus, error) {
	return m.status, m.statusErr
}

// Revoke returns the configured external flag or error.
//
// Parameters:
//   - ctx: ignored context.
//   - id: ignored secret id.
//   - manageHash: ignored manage hash.
//   - now: ignored current time.
//
// Returns: the configured revokeExternal flag and revoke error.
func (m storeMockIndex) Revoke(_ context.Context, _, _ string, _ time.Time) (bool, error) {
	return m.revokeExternal, m.revokeErr
}

// Claim returns the configured claim result or error.
//
// Parameters:
//   - ctx: ignored context.
//   - id: ignored secret id.
//   - claimHash: ignored claim hash.
//   - retry: ignored retry flag.
//   - now: ignored current time.
//   - claimedUntil: ignored lease deadline.
//   - opener: ignored external opener.
//
// Returns: the configured claim result and claim error, or app.ErrNotFound.
func (m storeMockIndex) Claim(_ context.Context, _ string, _ string, _ bool, _ time.Time, _ time.Time, _ store.ExternalOpener) (*store.IndexResult, error) {
	if m.claimErr != nil {
		return nil, m.claimErr
	}
	if m.claimResult != nil {
		return m.claimResult, nil
	}
	return nil, app.ErrNotFound
}

// Ack returns the configured external flag or error.
//
// Parameters:
//   - ctx: ignored context.
//   - id: ignored secret id.
//   - claimHash: ignored claim hash.
//
// Returns: the configured ackExternal flag and ack error.
func (m storeMockIndex) Ack(_ context.Context, _ string, _ string) (bool, error) {
	if m.ackErr != nil {
		return false, m.ackErr
	}
	return m.ackExternal, nil
}

// DeleteExpired returns the configured expired records.
//
// Parameters:
//   - ctx: ignored context.
//   - now: ignored cutoff time.
//
// Returns: the configured expired records and nil error.
func (m storeMockIndex) DeleteExpired(_ context.Context, _ time.Time) ([]store.ExpiredRecord, error) {
	return m.expired, nil
}

// ListExternalIDs returns configured external ids or a configured error.
//
// Parameters:
//   - ctx: ignored context.
//
// Returns: configured ids or a list error.
func (m storeMockIndex) ListExternalIDs(_ context.Context) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listIDs, nil
}

// storeCountingMetrics records counter increments by name.
type storeCountingMetrics struct{ counts map[string]int64 }

// Inc records a metric delta by counter name.
//
// Parameters:
//   - name: counter name.
//   - delta: amount to add.
//
// Returns: none.
func (c *storeCountingMetrics) Inc(name string, delta int64) { c.counts[name] += delta }
