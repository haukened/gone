// Package metrics provides a lightweight persistent metrics manager.
// It batches in-memory counter and summary observations and periodically
// flushes them to the shared SQLite database used for secrets. The design
// intentionally avoids dependencies and complex histogram logic; only
// monotonic counters and simple (count,sum,min,max) summaries are supported.
package metrics

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// Names for counters used by the application.
const (
	CounterSecretsCreated       = "secrets_created_total"
	CounterSecretsConsumed      = "secrets_consumed_total"
	CounterSecretsRevoked       = "secrets_revoked_total"
	CounterSecretsExpiredDelete = "secrets_expired_deleted_total"
	CounterRateLimitedCreate    = "rate_limited_create_total"
	CounterRateLimitedRead      = "rate_limited_read_total"
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
	summaries map[string]*SummaryAgg
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

// SummaryAgg stores the aggregate values for a summary metric.
type SummaryAgg struct {
	Count int64
	Sum   int64
	Min   int64
	Max   int64
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
		summaries: make(map[string]*SummaryAgg),
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
		// No loop running; apply queued events and flush.
		m.drain()
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
