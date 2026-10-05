package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

func TestStoreDeleteExpired(t *testing.T) {
	now := time.Now().UTC()
	fixture := storeNewFixture(t, now, 4)
	storeSave(t, fixture, "44444444444444444444444444444444", app.Meta{Version: 1, NonceB64u: "a"}, []byte("external-data"), now.Add(-5*time.Minute))
	storeSave(t, fixture, "55555555555555555555555555555555", app.Meta{Version: 1, NonceB64u: "b"}, []byte("inl"), now.Add(-5*time.Minute))
	storeSave(t, fixture, "66666666666666666666666666666666", app.Meta{Version: 1, NonceB64u: "c"}, []byte("f"), now.Add(5*time.Minute))
	storeAssertBlobExists(t, fixture.blobDir, "44444444444444444444444444444444")
	count, err := fixture.store.DeleteExpired(fixture.ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 expired removed, got %d", count)
	}
	storeAssertBlobMissing(t, fixture.blobDir, "44444444444444444444444444444444")
	if _, err := fixture.store.Claim(fixture.ctx, "66666666666666666666666666666666", "future-hash", false, now.Add(time.Minute)); err != nil {
		t.Fatalf("future claim: %v", err)
	}
}

func TestStoreDeleteExpiredCountsClaimMetrics(t *testing.T) {
	metrics := &storeCountingMetrics{counts: map[string]int64{}}
	st := store.New(storeMockIndex{expired: []store.ExpiredRecord{
		{ID: "claimed-inline", Claimed: true},
		{ID: "claimed-external", External: true, Claimed: true},
		{ID: "ttl-external", External: true},
	}}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 4).WithMetrics(metrics)
	count, err := st.DeleteExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if count != 3 {
		t.Fatalf("count got=%d want=3", count)
	}
	if got := metrics.counts[store.CounterClaimsExpired]; got != 2 {
		t.Fatalf("claimed-expired metric got=%d want=2", got)
	}
}

func TestStoreReconcileDeletesOrphan(t *testing.T) {
	now := time.Now().UTC()
	fixture := storeNewFixture(t, now, 4)
	id := "77777777777777777777777777777777"
	storeWriteTempBlob(t, fixture.blobDir, id, []byte("zzz"))
	time.Sleep(1100 * time.Millisecond)
	if err := fixture.store.Reconcile(fixture.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	storeAssertBlobMissing(t, fixture.blobDir, id)
}
