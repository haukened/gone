package sqlite

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
)

func TestIndexClaimExternalConcurrentOnlyOneFreshClaimSucceeds(t *testing.T) {
	db := sqliteOpenTestDB(t)
	ix, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	id := "ext-concurrent"
	now := time.Now().UTC()
	sqliteInsertExternalSecret(ctx, t, ix, id, 4, now, now.Add(time.Minute))
	errs, opens := sqliteRunConcurrentExternalClaims(ctx, ix, id, now)
	sqliteAssertConcurrentClaimResults(t, errs)
	if got := atomic.LoadInt32(opens); got != 1 {
		t.Fatalf("expected exactly one external open, got %d", got)
	}
}

type sqliteConcurrentClaimJob struct {
	ctx   context.Context
	ix    *Index
	id    string
	now   time.Time
	start <-chan struct{}
	errs  chan<- error
	opens *int32
}

// sqliteRunConcurrentExternalClaims runs two fresh claims for the same external row.
//
// Parameters:
//   - ctx: request context.
//   - ix: index under test.
//   - id: secret id to claim.
//   - now: current time used for claim checks.
//
// Returns: both claim errors and a pointer to the external open counter.
func sqliteRunConcurrentExternalClaims(ctx context.Context, ix *Index, id string, now time.Time) ([]error, *int32) {
	var opens int32
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	job := sqliteConcurrentClaimJob{ctx: ctx, ix: ix, id: id, now: now, start: start, errs: errs, opens: &opens}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go sqliteConcurrentClaimWorker(job, i, &wg)
	}
	close(start)
	wg.Wait()
	close(errs)
	return []error{<-errs, <-errs}, &opens
}

// sqliteConcurrentClaimWorker performs one concurrent claim attempt.
//
// Parameters:
//   - job: shared claim dependencies.
//   - n: worker number used to derive a claim hash.
//   - wg: wait group decremented when the worker exits.
//
// Returns: none; the claim error is sent on errs.
func sqliteConcurrentClaimWorker(job sqliteConcurrentClaimJob, n int, wg *sync.WaitGroup) {
	defer wg.Done()
	<-job.start
	res, err := job.ix.Claim(job.ctx, job.id, "hash-concurrent-"+string(rune('a'+n)), false, job.now.Add(time.Second), job.now.Add(time.Minute), func(string) (io.ReadCloser, error) {
		atomic.AddInt32(job.opens, 1)
		time.Sleep(50 * time.Millisecond)
		return io.NopCloser(bytes.NewReader([]byte("blob"))), nil
	})
	if err == nil && res != nil && res.Reader != nil {
		_ = res.Reader.Close()
	}
	job.errs <- err
}

// sqliteAssertConcurrentClaimResults verifies one success and one not found.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - errs: errors returned by the concurrent claim attempts.
//
// Returns: none; failures abort the test.
func sqliteAssertConcurrentClaimResults(t *testing.T, errs []error) {
	t.Helper()
	successes := 0
	notFound := 0
	for _, err := range errs {
		success, missing := sqliteClassifyClaimError(t, err)
		if success {
			successes++
		}
		if missing {
			notFound++
		}
	}
	if successes != 1 || notFound != 1 {
		t.Fatalf("expected one success and one not found, got success=%d not_found=%d", successes, notFound)
	}
}

// sqliteClassifyClaimError classifies a concurrent claim result.
//
// Parameters:
//   - t: test handle used for failure reporting.
//   - err: claim error to classify.
//
// Returns: whether the claim succeeded and whether it returned app.ErrNotFound.
func sqliteClassifyClaimError(t *testing.T, err error) (bool, bool) {
	t.Helper()
	if err == nil {
		return true, false
	}
	if errors.Is(err, app.ErrNotFound) {
		return false, true
	}
	t.Fatalf("unexpected claim error: %v", err)
	return false, false
}
