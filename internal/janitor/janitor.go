// Package janitor implements background cleanup of expired secrets and orphan blobs.
// It operates independently from the main app Service to keep lifecycle concerns
// (periodic deletion, reconciliation) isolated from request path logic.
package janitor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Store abstracts the minimal store operations the Janitor requires after simplification.
// We deliberately avoid exposing batch semantics; the underlying store internally handles
// expired secret deletion (including best-effort blob cleanup) and whole-store reconciliation
// (orphan blob removal).
type Store interface {
	// DeleteExpired deletes secrets whose expiry is <= t and returns the number removed.
	DeleteExpired(ctx context.Context, t time.Time) (int, error)
	// Reconcile performs orphan blob cleanup (best-effort) and may return an error if the
	// reconciliation scan itself fails.
	Reconcile(ctx context.Context) error
}

// Config holds tunables for the Janitor.
type Config struct {
	Interval time.Duration // how often a cycle begins
	// BatchSize kept for backward compatibility/no-op to avoid breaking existing callers.
	BatchSize int          // (deprecated) ignored; retained to prevent widespread refactors
	Logger    *slog.Logger // optional logger (defaults to slog.Default())
}

// Metrics accumulates counters (in-memory) for operational insight.
// Exposed fields kept simple for future export integration.
type Metrics struct {
	mu                  sync.Mutex
	Cycles              uint64
	Deleted             uint64
	Processed           uint64
	CycleLastDurationMS int64
}

// MetricsView is a read-only snapshot safe to copy.
type MetricsView struct {
	Cycles              uint64
	Deleted             uint64
	Processed           uint64
	CycleLastDurationMS int64
}

func (m *Metrics) addProcessed(n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	m.Processed += uint64(n)
	m.mu.Unlock()
}
func (m *Metrics) addDeleted(n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	m.Deleted += uint64(n)
	m.mu.Unlock()
}
func (m *Metrics) recordCycle(d time.Duration) {
	m.mu.Lock()
	m.Cycles++
	m.CycleLastDurationMS = d.Milliseconds()
	m.mu.Unlock()
}

// Janitor encapsulates the background cleanup loop.
type Janitor struct {
	store   Store
	cfg     Config
	metrics *Metrics

	ticker *time.Ticker
	stopCh chan struct{}
	doneCh chan struct{}
	once   sync.Once
}

// New constructs but does not start a Janitor.
func New(store Store, _ interface{}, cfg Config) *Janitor { // second param kept to preserve call sites; ignored
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Janitor{
		store:   store,
		cfg:     cfg,
		metrics: &Metrics{},
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start launches the janitor loop in a new goroutine.
func (j *Janitor) Start(ctx context.Context) {
	if j.ticker != nil {
		return
	} // already started
	j.ticker = time.NewTicker(j.cfg.Interval)
	go j.loop(ctx)
}

// Stop signals the loop to exit and waits for completion.
func (j *Janitor) Stop() {
	j.once.Do(func() { close(j.stopCh) })
	<-j.doneCh
}

// MetricsSnapshot returns a copy of current metrics.
func (j *Janitor) MetricsSnapshot() MetricsView {
	j.metrics.mu.Lock()
	defer j.metrics.mu.Unlock()
	return MetricsView{
		Cycles:              j.metrics.Cycles,
		Deleted:             j.metrics.Deleted,
		Processed:           j.metrics.Processed,
		CycleLastDurationMS: j.metrics.CycleLastDurationMS,
	}
}

func (j *Janitor) loop(ctx context.Context) {
	log := j.cfg.Logger.With("domain", "janitor")
	defer func() {
		if j.ticker != nil {
			j.ticker.Stop()
		}
		close(j.doneCh)
	}()
	for {
		select {
		case <-ctx.Done():
			log.Info("janitor stop", "reason", "context_cancel")
			return
		case <-j.stopCh:
			log.Info("janitor stop", "reason", "stop_signal")
			return
		case <-j.ticker.C:
			j.runCycle(ctx)
		}
	}
}

// runCycle performs one full expiry + orphan cleanup cycle.
func (j *Janitor) runCycle(ctx context.Context) {
	start := time.Now()
	log := j.cfg.Logger.With("domain", "janitor", "action", "cycle")
	now := time.Now().UTC()
	count, err := j.store.DeleteExpired(ctx, now)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("expire", "error", err)
	}
	if rerr := j.store.Reconcile(ctx); rerr != nil && !errors.Is(rerr, context.Canceled) {
		log.Error("reconcile", "error", rerr)
	}
	j.metrics.addProcessed(count)
	j.metrics.addDeleted(count)
	// Orphan count unknown with simplified Reconcile; skip addOrphans.
	j.metrics.recordCycle(time.Since(start))
	log.Info("cycle complete", "processed", count, "deleted", count, "ms", time.Since(start).Milliseconds())
}

// NOTE: Simplified implementation: batch semantics removed. Revisit only if future
// scale requires incremental draining to reduce lock contention.
