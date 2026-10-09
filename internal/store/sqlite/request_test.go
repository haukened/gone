package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

const (
	reqFill   = "66696c6c2d68617368"
	reqManage = "6d616e6167652d68617368"
)

// reqFixture opens an Index with one open request "r" created at now,
// expiring after an hour, whose reply is kept for 30 minutes.
func reqFixture(t *testing.T) (*Index, *sql.DB, time.Time) {
	t.Helper()
	ix, db := sqliteNewManageIndex(t)
	now := time.Unix(1700000000, 0).UTC()
	err := ix.InsertRequest(context.Background(), store.NewRequestRow{
		ID: "r", FillHash: reqFill, ManageHash: reqManage, TTL: 30 * time.Minute,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertRequest: %v", err)
	}
	return ix, db, now
}

func reqReply() store.NewRow {
	return store.NewRow{Meta: app.Meta{Version: 3, NonceB64u: "nonce"}, Inline: []byte("ciphertext"), Size: 10}
}

func reqCount(t *testing.T, db *sql.DB, table, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id=?`, id).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestRequestOpen(t *testing.T) {
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	exp, err := ix.RequestOpen(ctx, "r", reqFill, now)
	if err != nil || !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("RequestOpen = %v, %v", exp, err)
	}
	cases := []struct {
		name, id, hash string
		at             time.Time
	}{
		{"wrong fill", "r", "bad", now},
		{"manage hash is not a fill hash", "r", reqManage, now},
		{"expired", "r", reqFill, now.Add(time.Hour)},
		{"unknown", "nope", reqFill, now},
	}
	for _, c := range cases {
		if _, err := ix.RequestOpen(ctx, c.id, c.hash, c.at); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestFillRequestBecomesReply(t *testing.T) {
	ix, db, now := reqFixture(t)
	ctx := context.Background()
	at := now.Add(50 * time.Minute)
	exp, err := ix.FillRequest(ctx, "r", reqFill, at, reqReply())
	if err != nil || !exp.Equal(at.Add(30*time.Minute)) {
		t.Fatalf("FillRequest = %v, %v", exp, err)
	}
	if reqCount(t, db, "requests", "r") != 0 || reqCount(t, db, "secrets", "r") != 1 {
		t.Fatalf("request not moved to secrets")
	}
	if _, err := ix.FillRequest(ctx, "r", reqFill, at, reqReply()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("second fill err = %v", err)
	}
	st, err := ix.RequestStatus(ctx, "r", reqManage, at)
	if err != nil || !st.Ready || !st.ExpiresAt.Equal(exp) {
		t.Fatalf("status after fill = %+v, %v", st, err)
	}
	// A reply is never claimable through the secret route.
	if _, err := ix.Claim(ctx, "r", "claim", false, at, at.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("secret claim of reply err = %v", err)
	}
	res, err := ix.ClaimReply(ctx, "r", reqManage, "claim", false, at, at.Add(time.Minute), nil)
	if err != nil || string(res.Inline) != "ciphertext" || res.Meta.Version != 3 {
		t.Fatalf("ClaimReply = %+v, %v", res, err)
	}
	if _, err := ix.Ack(ctx, "r", "claim"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if _, err := ix.RequestStatus(ctx, "r", reqManage, at); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("status after ack err = %v", err)
	}
}

func TestFillRequestRejects(t *testing.T) {
	ix, db, now := reqFixture(t)
	ctx := context.Background()
	if _, err := ix.FillRequest(ctx, "r", "bad", now, reqReply()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("wrong fill err = %v", err)
	}
	if _, err := ix.FillRequest(ctx, "r", reqFill, now.Add(time.Hour), reqReply()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expired fill err = %v", err)
	}
	if _, err := ix.FillRequest(ctx, "nope", reqFill, now, reqReply()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown fill err = %v", err)
	}
	if reqCount(t, db, "requests", "r") != 1 || reqCount(t, db, "secrets", "r") != 0 {
		t.Fatalf("rejected fill changed state")
	}
}

func TestFillRequestConcurrentExactlyOne(t *testing.T) {
	ix, _, now := reqFixture(t)
	const n = 8
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ix.FillRequest(context.Background(), "r", reqFill, now, reqReply()); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("successful fills = %d, want 1", ok)
	}
}

func TestRequestStatus(t *testing.T) {
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	st, err := ix.RequestStatus(ctx, "r", reqManage, now)
	if err != nil || st.Ready || !st.CreatedAt.Equal(now) {
		t.Fatalf("waiting status = %+v, %v", st, err)
	}
	for _, c := range []struct {
		name, id, hash string
		at             time.Time
	}{
		{"wrong manage", "r", "bad", now},
		{"fill hash is not a manage hash", "r", reqFill, now},
		{"expired", "r", reqManage, now.Add(time.Hour)},
		{"unknown", "nope", reqManage, now},
	} {
		if _, err := ix.RequestStatus(ctx, c.id, c.hash, c.at); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	// An ordinary secret is never reported as a request reply.
	sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("x"), now, now.Add(time.Hour))
	if _, err := ix.RequestStatus(ctx, "s", sqliteTestManageHash, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("secret as request err = %v", err)
	}
}

func TestRequestStatusReplyExpired(t *testing.T) {
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	if _, err := ix.FillRequest(ctx, "r", reqFill, now, reqReply()); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.RequestStatus(ctx, "r", "bad", now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("wrong manage on reply err = %v", err)
	}
	if _, err := ix.RequestStatus(ctx, "r", reqManage, now.Add(30*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expired reply err = %v", err)
	}
}

func TestClaimReplyRejects(t *testing.T) {
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	if _, err := ix.ClaimReply(ctx, "r", reqManage, "claim", false, now, now.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("claim of open request err = %v", err)
	}
	if _, err := ix.FillRequest(ctx, "r", reqFill, now, reqReply()); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.ClaimReply(ctx, "r", "bad", "claim", false, now, now.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("wrong manage claim err = %v", err)
	}
	sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("x"), now, now.Add(time.Hour))
	if _, err := ix.ClaimReply(ctx, "s", sqliteTestManageHash, "claim", false, now, now.Add(time.Minute), nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("secret via ClaimReply err = %v", err)
	}
}

func TestCancelRequest(t *testing.T) {
	ix, db, now := reqFixture(t)
	ctx := context.Background()
	if _, err := ix.CancelRequest(ctx, "r", "bad", now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("wrong manage err = %v", err)
	}
	if _, err := ix.CancelRequest(ctx, "r", reqManage, now.Add(time.Hour)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expired err = %v", err)
	}
	ext, err := ix.CancelRequest(ctx, "r", reqManage, now)
	if err != nil || ext || reqCount(t, db, "requests", "r") != 0 {
		t.Fatalf("cancel open = %v, %v", ext, err)
	}
	if _, err := ix.CancelRequest(ctx, "r", reqManage, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("repeat cancel err = %v", err)
	}
}

func TestCancelRequestReplyWinsOverClaim(t *testing.T) {
	ix, db, now := reqFixture(t)
	ctx := context.Background()
	reply := reqReply()
	reply.Inline, reply.External = nil, true
	if _, err := ix.FillRequest(ctx, "r", reqFill, now, reply); err != nil {
		t.Fatal(err)
	}
	ext, err := ix.CancelRequest(ctx, "r", reqManage, now)
	if err != nil || !ext || reqCount(t, db, "secrets", "r") != 0 {
		t.Fatalf("cancel reply = %v, %v", ext, err)
	}
	// An ordinary secret cannot be cancelled as a request.
	sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("x"), now, now.Add(time.Hour))
	if _, err := ix.CancelRequest(ctx, "s", sqliteTestManageHash, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("cancel secret err = %v", err)
	}
}

func TestDeleteExpiredSweepsRequests(t *testing.T) {
	ix, db, now := reqFixture(t)
	ctx := context.Background()
	recs, err := ix.DeleteExpired(ctx, now.Add(59*time.Minute))
	if err != nil || len(recs) != 0 {
		t.Fatalf("early sweep = %v, %v", recs, err)
	}
	recs, err = ix.DeleteExpired(ctx, now.Add(time.Hour))
	if err != nil || len(recs) != 1 || !recs[0].Request || recs[0].ID != "r" {
		t.Fatalf("sweep = %+v, %v", recs, err)
	}
	if reqCount(t, db, "requests", "r") != 0 {
		t.Fatalf("expired request not deleted")
	}
}

func TestRequestQueriesFailOnClosedDB(t *testing.T) {
	ix, db, now := reqFixture(t)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.RequestOpen(ctx, "r", reqFill, now); err == nil || errors.Is(err, app.ErrNotFound) {
		t.Errorf("RequestOpen err = %v", err)
	}
	if _, err := ix.RequestStatus(ctx, "r", reqManage, now); err == nil || errors.Is(err, app.ErrNotFound) {
		t.Errorf("RequestStatus err = %v", err)
	}
	if err := ix.InsertRequest(ctx, store.NewRequestRow{ID: "x"}); err == nil {
		t.Errorf("InsertRequest on closed db succeeded")
	}
	if _, err := ix.FillRequest(ctx, "r", reqFill, now, reqReply()); err == nil {
		t.Errorf("FillRequest on closed db succeeded")
	}
	if _, err := ix.CancelRequest(ctx, "r", reqManage, now); err == nil {
		t.Errorf("CancelRequest on closed db succeeded")
	}
	if _, err := ix.DeleteExpired(ctx, now); err == nil {
		t.Errorf("DeleteExpired on closed db succeeded")
	}
}

// TestRequestPollingTakesNoWriteTurn holds the write gate and checks that the
// open check and status still answer at once: polling never queues behind,
// or adds to, writes.
func TestRequestPollingTakesNoWriteTurn(t *testing.T) {
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	release, err := ix.acquireWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 2)
	go func() { _, err := ix.RequestOpen(ctx, "r", reqFill, now); done <- err }()
	go func() { _, err := ix.RequestStatus(ctx, "r", reqManage, now); done <- err }()
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("read while gate held: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("polling waited for the write gate")
		}
	}
}
