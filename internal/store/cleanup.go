package store

import (
	"context"
	"errors"
	"time"
)

// CounterClaimsExpired counts secrets that were claimed but never
// acknowledged before their lease lapsed.
const CounterClaimsExpired = "secrets_claims_expired_total"

// CounterRequestsExpired counts secret requests whose reply window closed
// without an answer. DeleteExpired reports them here rather than in its
// returned count, which covers secrets only.
const CounterRequestsExpired = "requests_expired_total"

// DeleteExpired removes secrets whose expiry is <= t or whose claim lease has
// lapsed without acknowledgement, and returns the count. Expired open
// requests are removed too and counted in CounterRequestsExpired.
// Blob files for expired records are removed best-effort.
func (s *Store) DeleteExpired(ctx context.Context, t time.Time) (int, error) {
	expired, err := s.index.DeleteExpired(ctx, t)
	if err != nil {
		return 0, err
	}
	claimed, requests := 0, 0
	for _, rec := range expired {
		if rec.External {
			_ = s.blobs.Delete(rec.ID) // best-effort
		}
		if rec.Claimed {
			claimed++
		}
		if rec.Request {
			requests++
		}
	}
	s.count(CounterClaimsExpired, claimed)
	s.count(CounterRequestsExpired, requests)
	return len(expired) - requests, nil
}

// count adds n to a counter when metrics are configured and n is positive.
//
// Parameters:
//   - name: counter name.
//   - n: amount to add.
func (s *Store) count(name string, n int) {
	if s.metrics != nil && n > 0 {
		s.metrics.Inc(name, int64(n))
	}
}

// Reconcile scans for blob orphans and removes them. A blob whose save is
// still in flight, or was at any point during the scan, is never an orphan
// here even though its row is not visible yet (see blobGuard).
func (s *Store) Reconcile(ctx context.Context) error {
	if s.index == nil || s.blobs == nil {
		return errors.New("store not properly initialized")
	}
	s.guard.beginScan()
	defer s.guard.endScan()
	blobIDs, err := s.blobs.List()
	if err != nil {
		return err
	}
	extIDs, err := s.index.ListExternalIDs(ctx)
	if err != nil {
		return err
	}
	s.deleteReconcileOrphans(blobIDs, extIDs)
	return nil
}

// deleteReconcileOrphans removes blob IDs not present in the external index
// set, skipping any the guard saw saved during the scan.
//
// Parameters:
//   - blobIDs: IDs currently present in blob storage.
//   - extIDs: IDs recorded as external in the index.
func (s *Store) deleteReconcileOrphans(blobIDs, extIDs []string) {
	indexSet := externalIDMembership(extIDs)
	for _, bid := range blobIDs {
		if _, ok := indexSet[bid]; !ok {
			s.guard.deleteUntouched(bid, s.blobs.Delete)
		}
	}
}

// externalIDMembership builds a lookup set for externally stored secret IDs.
//
// Parameters:
//   - ids: external IDs read from the index.
//
// Returns a set keyed by secret ID.
func externalIDMembership(ids []string) map[string]struct{} {
	indexSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		indexSet[id] = struct{}{}
	}
	return indexSet
}
