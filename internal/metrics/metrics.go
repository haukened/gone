// Package metrics provides a lightweight persistent metrics manager.
// It batches in-memory counter and summary observations and periodically
// flushes them to the shared SQLite database used for secrets. The design
// intentionally avoids dependencies and complex histogram logic; only
// monotonic counters and simple (count,sum,min,max) summaries are supported.
package metrics

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Names for counters used by the application.
const (
	CounterSecretsCreated       = "secrets_created_total"
	CounterSecretsConsumed      = "secrets_consumed_total"
	CounterSecretsExpiredDelete = "secrets_expired_deleted_total"
	// Future: CounterOrphanBlobsDeleted = "secrets_orphan_blobs_deleted_total"
)

// Summary names.
const (
	SummaryJanitorDeletedPerCycle = "janitor_deleted_per_cycle"
)

// Config controls flush cadence and logging.
type Config struct {
	FlushInterval time.Duration
	Logger        *slog.Logger
}

// Manager aggregates metric events and flushes them.
type Manager struct {
	cfg     Config
	db      *sql.DB
	events  chan event
	stop    chan struct{}
	done    chan struct{}
	started bool

	// in-memory deltas (protected by mu)
	mu        sync.Mutex
	counters  map[string]int64
	summaries map[string]*summaryAgg
}

type eventKind int

const (
	eventInc eventKind = iota + 1
	eventObserve
)

type event struct {
	kind eventKind
	name string
	v    int64
}

type summaryAgg struct {
	count int64
	sum   int64
	min   int64
	max   int64
}

// New creates a Manager. Call Start to begin background flushing.
func New(db *sql.DB, cfg Config) *Manager {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	m := &Manager{
		cfg:       cfg,
		db:        db,
		events:    make(chan event, 1024),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		counters:  make(map[string]int64),
		summaries: make(map[string]*summaryAgg),
	}
	return m
}

// InitSchema ensures metrics tables exist.
func (m *Manager) InitSchema(ctx context.Context) error {
	ddlCounters := `CREATE TABLE IF NOT EXISTS metrics_counters (
		name TEXT PRIMARY KEY,
		value INTEGER NOT NULL
	);`
	ddlSummaries := `CREATE TABLE IF NOT EXISTS metrics_summaries (
		name TEXT PRIMARY KEY,
		count INTEGER NOT NULL,
		sum INTEGER NOT NULL,
		min INTEGER NOT NULL,
		max INTEGER NOT NULL
	);`
	if _, err := m.db.ExecContext(ctx, ddlCounters); err != nil {
		return err
	}
	if _, err := m.db.ExecContext(ctx, ddlSummaries); err != nil {
		return err
	}
	return nil
}

// Start launches the background flush loop.
func (m *Manager) Start(ctx context.Context) {
	if m.started {
		return
	}
	m.started = true
	go m.loop(ctx)
}

// Stop signals flush loop to exit and performs a final flush.
func (m *Manager) Stop(ctx context.Context) {
	if !m.started {
		// No loop running; just flush any deltas.
		_ = m.flush(ctx)
		return
	}
	close(m.stop)
	<-m.done
	_ = m.flush(ctx)
}

// Inc increments a counter by delta (>=1).
func (m *Manager) Inc(name string, delta int64) {
	if delta <= 0 {
		return
	}
	select {
	case m.events <- event{kind: eventInc, name: name, v: delta}:
	default:
		// channel full; best-effort drop (could add a dropped counter later)
	}
}

// Observe records a summary observation.
func (m *Manager) Observe(name string, value int64) {
	select {
	case m.events <- event{kind: eventObserve, name: name, v: value}:
	default:
	}
}

func (m *Manager) loop(ctx context.Context) {
	log := m.cfg.Logger.With("domain", "metrics")
	Ticker := time.NewTicker(m.cfg.FlushInterval)
	defer func() {
		Ticker.Stop()
		close(m.done)
	}()
	for {
		select {
		case <-ctx.Done():
			log.Info("metrics stop", "reason", "context_cancel")
			return
		case <-m.stop:
			log.Info("metrics stop", "reason", "stop_signal")
			return
		case ev := <-m.events:
			m.apply(ev)
		case <-Ticker.C:
			if err := m.flush(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("flush", "error", err)
			}
		}
	}
}

func (m *Manager) apply(ev event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ev.kind {
	case eventInc:
		m.counters[ev.name] += ev.v
	case eventObserve:
		agg := m.summaries[ev.name]
		if agg == nil {
			agg = &summaryAgg{count: 1, sum: ev.v, min: ev.v, max: ev.v}
			m.summaries[ev.name] = agg
			return
		}
		agg.count++
		agg.sum += ev.v
		if ev.v < agg.min {
			agg.min = ev.v
		}
		if ev.v > agg.max {
			agg.max = ev.v
		}
	}
}

// Snapshot returns current (persisted + in-memory deltas) by reading persisted
// state and layering deltas. This is optional and may be refined later.
func (m *Manager) Snapshot(ctx context.Context) (counters map[string]int64, summaries map[string]summaryAgg, err error) {
	counters = make(map[string]int64)
	summaries = make(map[string]summaryAgg)
	// Load persisted counters.
	rows, err := m.db.QueryContext(ctx, `SELECT name, value FROM metrics_counters`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		var v int64
		if err := rows.Scan(&n, &v); err != nil {
			return nil, nil, err
		}
		counters[n] = v
	}
	// Load persisted summaries.
	srows, err := m.db.QueryContext(ctx, `SELECT name, count, sum, min, max FROM metrics_summaries`)
	if err != nil {
		return nil, nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var n string
		var c, s, mn, mx int64
		if err := srows.Scan(&n, &c, &s, &mn, &mx); err != nil {
			return nil, nil, err
		}
		summaries[n] = summaryAgg{count: c, sum: s, min: mn, max: mx}
	}
	// Layer deltas.
	m.mu.Lock()
	for n, v := range m.counters {
		counters[n] += v
	}
	for n, agg := range m.summaries {
		cur := summaries[n]
		if cur.count == 0 {
			summaries[n] = *agg
			continue
		}
		cur.count += agg.count
		cur.sum += agg.sum
		if agg.min < cur.min {
			cur.min = agg.min
		}
		if agg.max > cur.max {
			cur.max = agg.max
		}
		summaries[n] = cur
	}
	m.mu.Unlock()
	return counters, summaries, nil
}

// flush writes in-memory deltas to SQLite in a single transaction and resets them.
func (m *Manager) flush(ctx context.Context) error {
	m.mu.Lock()
	if len(m.counters) == 0 && len(m.summaries) == 0 {
		m.mu.Unlock()
		return nil
	}
	// Copy & reset.
	cCopy := make(map[string]int64, len(m.counters))
	for k, v := range m.counters {
		cCopy[k] = v
	}
	sCopy := make(map[string]*summaryAgg, len(m.summaries))
	for k, v := range m.summaries {
		cp := *v
		sCopy[k] = &cp
	}
	m.counters = make(map[string]int64)
	m.summaries = make(map[string]*summaryAgg)
	m.mu.Unlock()

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Upsert counters.
	for name, delta := range cCopy {
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_counters(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value = value + excluded.value`, name, delta); err != nil {
			tx.Rollback()
			return err
		}
	}
	// Upsert summaries.
	for name, agg := range sCopy {
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_summaries(name,count,sum,min,max) VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET count = metrics_summaries.count + excluded.count, sum = metrics_summaries.sum + excluded.sum, min = MIN(metrics_summaries.min, excluded.min), max = MAX(metrics_summaries.max, excluded.max)`, name, agg.count, agg.sum, agg.min, agg.max); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
