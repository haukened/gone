package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

func TestStoreNilReceiverClaim(t *testing.T) {
	var s *store.Store
	if _, err := s.Claim(context.Background(), "any", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error on nil store Claim")
	}
}

func TestStoreNilReceiverAck(t *testing.T) {
	var s *store.Store
	if err := s.Ack(context.Background(), "any", "hash"); err == nil {
		t.Fatalf("expected error on nil store Ack")
	}
}

func TestStoreNilReceiverSave(t *testing.T) {
	var s *store.Store
	if err := s.Save(context.Background(), "id", app.Meta{}, storeTestManageHash, storeBytesReader([]byte("a")), 1, time.Now()); err == nil {
		t.Fatalf("expected error on nil store Save")
	}
}

func TestStoreNilIndex(t *testing.T) {
	bs := &storeMockBlobStore{}
	s := store.New(nil, bs, storeFixedClock{now: time.Now()}, 10)
	if _, err := s.Claim(context.Background(), "x", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error with nil index")
	}
	if err := s.Ack(context.Background(), "x", "hash"); err == nil {
		t.Fatalf("expected ack error with nil index")
	}
}

func TestStoreNilBlobStorage(t *testing.T) {
	s := store.New(storeMockIndex{}, nil, storeFixedClock{now: time.Now()}, 10)
	if _, err := s.Claim(context.Background(), "x", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error with nil blob storage")
	}
	if err := s.Ack(context.Background(), "x", "hash"); err == nil {
		t.Fatalf("expected ack error with nil blob storage")
	}
}

func TestStoreNilClock(t *testing.T) {
	s := store.New(storeMockIndex{}, &storeMockBlobStore{}, nil, 10)
	if err := s.Save(context.Background(), "x", app.Meta{}, storeTestManageHash, storeBytesReader([]byte("a")), 1, time.Now()); err == nil {
		t.Fatalf("expected error with nil clock in Save")
	}
	if _, err := s.Claim(context.Background(), "x", "hash", false, time.Now()); err == nil {
		t.Fatalf("expected error with nil clock in Claim")
	}
}

func TestStoreSaveNegativeSize(t *testing.T) {
	s := store.New(storeMockIndex{}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 10)
	if err := s.Save(context.Background(), "x", app.Meta{}, storeTestManageHash, storeBytesReader([]byte("a")), -1, time.Now()); err == nil {
		t.Fatalf("expected error for negative size")
	}
}

func TestStoreReconcileNilIndex(t *testing.T) {
	s := store.New(nil, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected error with nil index in Reconcile")
	}
}

func TestStoreReconcileNilBlobs(t *testing.T) {
	s := store.New(storeMockIndex{}, nil, storeFixedClock{now: time.Now()}, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected error with nil blob storage in Reconcile")
	}
}

func TestStoreReconcileBlobListError(t *testing.T) {
	s := store.New(storeMockIndex{}, &storeMockBlobStore{listErr: errors.New("list boom")}, storeFixedClock{now: time.Now()}, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected list error propagated")
	}
}

func TestStoreReconcileIndexListError(t *testing.T) {
	s := store.New(storeMockIndex{listErr: errors.New("index list boom")}, &storeMockBlobStore{}, storeFixedClock{now: time.Now()}, 10)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatalf("expected index list error propagated")
	}
}

func TestStoreReconcileDeleteErrorIgnored(t *testing.T) {
	s := store.New(storeMockIndex{}, &storeMockBlobStore{listIDs: []string{"orphan"}, deleteErr: errors.New("del fail")}, storeFixedClock{now: time.Now()}, 10)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("unexpected error despite delete failure: %v", err)
	}
}
