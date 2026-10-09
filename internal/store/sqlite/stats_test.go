package sqlite

import (
	"context"
	"testing"
	"time"
)

// assertStats verifies Stats at the given time.
func assertStats(t *testing.T, ix *Index, at time.Time, want Stats) {
	t.Helper()
	got, err := ix.Stats(context.Background(), at)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got != want {
		t.Fatalf("Stats at %v = %+v, want %+v", at, got, want)
	}
}

func TestStatsEmpty(t *testing.T) {
	ix, _ := sqliteNewManageIndex(t)
	assertStats(t, ix, time.Now(), Stats{})
}

func TestStatsCountsLiveRows(t *testing.T) {
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	sqliteInsertInlineSecret(ctx, t, ix, "a", []byte("12345"), now, now.Add(time.Hour))
	sqliteInsertExternalSecret(ctx, t, ix, "b", 100, now, now.Add(time.Hour))
	sqliteInsertInlineSecret(ctx, t, ix, "c", []byte("gone"), now.Add(-time.Hour), now) // TTL ends now
	assertStats(t, ix, now, Stats{Secrets: 2, SecretBytes: 105, OpenRequests: 1})

	// Filling the request turns it into a 10-byte reply secret.
	if _, err := ix.FillRequest(ctx, "r", reqFill, now, reqReply()); err != nil {
		t.Fatalf("FillRequest: %v", err)
	}
	assertStats(t, ix, now, Stats{Secrets: 3, SecretBytes: 115})

	// A claim whose lease lapses without an ack no longer counts.
	if _, err := ix.Claim(ctx, "a", "claim", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	assertStats(t, ix, now.Add(2*time.Minute), Stats{Secrets: 2, SecretBytes: 110})
	assertStats(t, ix, now.Add(2*time.Hour), Stats{})
}

func TestStatsFailsOnClosedDB(t *testing.T) {
	ix, db, now := reqFixture(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Stats(context.Background(), now); err == nil {
		t.Fatal("Stats on closed db succeeded")
	}
}
