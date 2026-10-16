package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

// replyFixture returns an index holding a claimed reply row "r" (claim hash
// "claim") plus an ordinary secret "s", with the request's manage hash
// reqManage on the reply.
func replyFixture(t *testing.T) (*Index, time.Time) {
	t.Helper()
	ix, _, now := reqFixture(t)
	ctx := context.Background()
	if _, err := ix.FillRequest(ctx, "r", reqFill, now, reqReply()); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.ClaimReply(ctx, "r", reqManage, "claim", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	sqliteInsertInlineSecret(ctx, t, ix, "s", []byte("data"), now, now.Add(time.Hour))
	if _, err := ix.Claim(ctx, "s", "sclaim", false, now, now.Add(time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	return ix, now
}

// TestAckIgnoresReplyRows checks the secret route cannot ack a reply, and the
// reply route cannot ack a secret; neither deletes the row it refuses.
func TestAckIgnoresReplyRows(t *testing.T) {
	ix, _ := replyFixture(t)
	ctx := context.Background()
	if _, err := ix.Ack(ctx, "r", "claim"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Ack of a reply err = %v, want ErrNotFound", err)
	}
	if _, err := ix.AckReply(ctx, "s", "sclaim"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("AckReply of a secret err = %v, want ErrNotFound", err)
	}
	if _, err := ix.AckReply(ctx, "r", "wrong"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("AckReply wrong hash err = %v", err)
	}
	if _, err := ix.AckReply(ctx, "nope", "claim"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("AckReply unknown err = %v", err)
	}
	if _, err := ix.AckReply(ctx, "r", "claim"); err != nil {
		t.Fatalf("AckReply: %v", err)
	}
	if _, err := ix.Ack(ctx, "s", "sclaim"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

// TestStatusRevokeIgnoreReplyRows checks Status and Revoke (the secret route's
// manage operations) treat a reply as absent, while the request-side calls
// still reach it.
func TestStatusRevokeIgnoreReplyRows(t *testing.T) {
	ix, now := replyFixture(t)
	ctx := context.Background()
	if _, err := ix.Status(ctx, "r", reqManage, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Status of a reply err = %v, want ErrNotFound", err)
	}
	if _, err := ix.Revoke(ctx, "r", reqManage, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Revoke of a reply err = %v, want ErrNotFound", err)
	}
	if st, err := ix.RequestStatus(ctx, "r", reqManage, now); err != nil || !st.Ready {
		t.Fatalf("RequestStatus after refused Revoke = %+v, %v", st, err)
	}
	if _, err := ix.Status(ctx, "s", sqliteTestManageHash, now); err != nil {
		t.Fatalf("Status of a secret: %v", err)
	}
	if _, err := ix.CancelRequest(ctx, "r", reqManage, now); err != nil {
		t.Fatalf("CancelRequest of the reply: %v", err)
	}
	if _, err := ix.CancelRequest(ctx, "s", sqliteTestManageHash, now); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("CancelRequest of a secret err = %v, want ErrNotFound", err)
	}
	if _, err := ix.Revoke(ctx, "s", sqliteTestManageHash, now); err != nil {
		t.Fatalf("Revoke of a secret: %v", err)
	}
}
