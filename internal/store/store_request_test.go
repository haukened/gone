package store_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

const reqID = "0123456789abcdef0123456789abcdef"

const (
	reqFillHash   = "66696c6c"
	reqManageHash = "6d616e616765"
)

// reqCreate opens request "r" with a one-hour window and a 30-minute reply
// lifetime.
func reqCreate(t *testing.T, f storeFixture, now time.Time) {
	t.Helper()
	if err := f.store.CreateRequest(f.ctx, reqID, reqFillHash, reqManageHash, 30*time.Minute, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
}

func reqFill(f storeFixture, data []byte) (time.Time, error) {
	return f.store.Fill(f.ctx, reqID, reqFillHash, app.Meta{Version: 3, NonceB64u: "n"}, storeBytesReader(data), int64(len(data)))
}

func TestStoreRequestLifecycleExternal(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	f := storeNewFixture(t, now, 4)
	reqCreate(t, f, now)
	if exp, err := f.store.RequestOpen(f.ctx, reqID, reqFillHash); err != nil || !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("RequestOpen = %v, %v", exp, err)
	}
	if st, err := f.store.RequestStatus(f.ctx, reqID, reqManageHash); err != nil || st.Ready {
		t.Fatalf("status = %+v, %v", st, err)
	}
	exp, err := reqFill(f, []byte("larger than inline"))
	if err != nil || !exp.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("Fill = %v, %v", exp, err)
	}
	if _, err := os.Stat(filepath.Join(f.blobDir, reqID+".blob")); err != nil {
		t.Fatalf("blob not written: %v", err)
	}
	c, err := f.store.ClaimReply(f.ctx, reqID, reqManageHash, "claim", false, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimReply: %v", err)
	}
	body, _ := io.ReadAll(c.Body)
	_ = c.Body.Close()
	if string(body) != "larger than inline" {
		t.Fatalf("body = %q", body)
	}
	if err := f.store.Ack(f.ctx, reqID, "claim"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.blobDir, reqID+".blob")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blob not deleted: %v", err)
	}
}

func TestStoreFillClosedRequestWritesNoBlob(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	f := storeNewFixture(t, now, 4)
	if _, err := reqFill(f, []byte("larger than inline")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Fill unknown err = %v", err)
	}
	entries, _ := os.ReadDir(f.blobDir)
	if len(entries) != 0 {
		t.Fatalf("blob written for closed request: %v", entries)
	}
	if _, err := f.store.Fill(f.ctx, reqID, reqFillHash, app.Meta{}, storeBytesReader(nil), -1); err == nil {
		t.Fatalf("negative size accepted")
	}
}

func TestStoreCancelRequestDeletesReplyBlob(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	f := storeNewFixture(t, now, 4)
	reqCreate(t, f, now)
	if _, err := reqFill(f, []byte("larger than inline")); err != nil {
		t.Fatal(err)
	}
	if err := f.store.CancelRequest(f.ctx, reqID, reqManageHash); err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.blobDir, reqID+".blob")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blob not deleted: %v", err)
	}
	if err := f.store.CancelRequest(f.ctx, reqID, reqManageHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("repeat cancel err = %v", err)
	}
}

func TestStoreDeleteExpiredCountsRequestsSeparately(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	f := storeNewFixture(t, now, 64)
	m := &storeCountingMetrics{counts: map[string]int64{}}
	f.store.WithMetrics(m)
	reqCreate(t, f, now)
	n, err := f.store.DeleteExpired(f.ctx, now.Add(time.Hour))
	if err != nil || n != 0 {
		t.Fatalf("DeleteExpired = %d, %v", n, err)
	}
	if m.counts[store.CounterRequestsExpired] != 1 {
		t.Fatalf("counts = %v", m.counts)
	}
}

func TestStoreRequestsUnsupportedIndex(t *testing.T) {
	s := store.New(storeMockIndex{}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 1)
	checks := []error{
		s.CreateRequest(t.Context(), "r", "f", "m", time.Minute, time.Now()),
		s.CancelRequest(t.Context(), "r", "m"),
	}
	_, err := s.RequestOpen(t.Context(), "r", "f")
	checks = append(checks, err)
	_, err = s.Fill(t.Context(), "r", "f", app.Meta{}, storeBytesReader(nil), 0)
	checks = append(checks, err)
	_, err = s.RequestStatus(t.Context(), "r", "m")
	checks = append(checks, err)
	_, err = s.ClaimReply(t.Context(), "r", "m", "c", false, time.Now())
	checks = append(checks, err)
	for i, err := range checks {
		if err == nil {
			t.Errorf("call %d: want error for index without request support", i)
		}
	}
	var nilStore *store.Store
	if err := nilStore.CancelRequest(t.Context(), "r", "m"); err == nil {
		t.Errorf("nil store accepted")
	}
}
