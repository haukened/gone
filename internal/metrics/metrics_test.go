package metrics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// openTempDB creates an isolated sqlite database file for tests.
func openTempDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "m.db")
	db, err := sql.Open("sqlite3", p)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestManagerIncFlush(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	m.Inc(CounterSecretsCreated, 1)
	m.Inc(CounterSecretsCreated, 2)
	// give goroutine chance if it were started (not started here); events go straight into channel; drain manually
	// apply pending events by triggering flush (flush reads from memory after events consumed, so manually pull events)
	// Manually drain event channel since loop not running
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			goto drained
		}
	}
drained:
	// force flush early
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsCreated)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("no row for counter")
	}
	var v int64
	if err := rows.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v != 3 {
		t.Fatalf("expected 3 got %d", v)
	}
}

func TestManagerObserveFlushSnapshot(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: 500 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	m.Observe(SummaryJanitorDeletedPerCycle, 5)
	m.Observe(SummaryJanitorDeletedPerCycle, 7)
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			goto drained2
		}
	}
drained2:
	// flush
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	counters, summaries, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(counters) != 0 {
		t.Fatalf("unexpected counters %+v", counters)
	}
	agg, ok := summaries[SummaryJanitorDeletedPerCycle]
	if !ok {
		t.Fatalf("missing summary")
	}
	if agg.count != 2 || agg.sum != 12 || agg.min != 5 || agg.max != 7 {
		t.Fatalf("bad summary %+v", agg)
	}
}

func TestManagerSummaryLayering(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	// Seed persisted summary: count=3, sum=30, min=5, max=20
	if _, err := db.ExecContext(ctx, `INSERT INTO metrics_summaries(name,count,sum,min,max) VALUES(?,?,?,?,?)`, SummaryJanitorDeletedPerCycle, 3, 30, 5, 20); err != nil {
		t.Fatalf("seed summary: %v", err)
	}
	// In-memory observations: values 4 (should update min to 4), 25 (should update max to 25), 6 (between)
	m.Observe(SummaryJanitorDeletedPerCycle, 4)
	m.Observe(SummaryJanitorDeletedPerCycle, 25)
	m.Observe(SummaryJanitorDeletedPerCycle, 6)
	// Drain pending events into in-memory aggregates
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			goto drainedSummary
		}
	}
drainedSummary:
	counters, summaries, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(counters) != 0 {
		t.Fatalf("unexpected counters %+v", counters)
	}
	agg, ok := summaries[SummaryJanitorDeletedPerCycle]
	if !ok {
		t.Fatalf("missing summary layering result")
	}
	// Expected: new count = 3 + 3 = 6; sum = 30 + (4+25+6)=65; min becomes 4; max becomes 25
	if agg.count != 6 || agg.sum != 65 || agg.min != 4 || agg.max != 25 {
		t.Fatalf("unexpected layered summary %+v", agg)
	}
}

func TestManagerStopFinalFlush(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: time.Hour}) // long interval so we rely on Stop
	ctx, cancel := context.WithCancel(context.Background())
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	m.Inc(CounterSecretsConsumed, 4)
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			goto drained3
		}
	}
drained3:
	// stop triggers final flush
	m.Stop(context.Background())
	cancel()
	row := db.QueryRowContext(context.Background(), `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsConsumed)
	var v int64
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v != 4 {
		t.Fatalf("expected 4 got %d", v)
	}
}

func TestManagerSnapshotMergesDeltas(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	// Persist some data manually to simulate prior runs.
	if _, err := db.ExecContext(ctx, `INSERT INTO metrics_counters(name,value) VALUES(?,10)`, CounterSecretsCreated); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m.Inc(CounterSecretsCreated, 5)
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			goto drained4
		}
	}
drained4:
	cnt, _, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if cnt[CounterSecretsCreated] != 15 {
		t.Fatalf("expected merged 15 got %d", cnt[CounterSecretsCreated])
	}
}

// Ensure flush with empty state is a fast no-op
func TestManagerFlushEmpty(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush empty: %v", err)
	}
	// basic sanity: no panic and empty flush succeeded
}

func TestManagerStartIdempotent(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	m.Start(ctx)
	m.Start(ctx) // second call should be no-op
	// emit and allow loop to consume
	m.Inc(CounterSecretsCreated, 1)
	time.Sleep(20 * time.Millisecond) // allow flush ticker at least once
	cancel()
	m.Stop(context.Background())
	row := db.QueryRowContext(context.Background(), `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsCreated)
	var v int64
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v == 0 {
		t.Fatalf("expected counter increment persisted")
	}
}

func TestManagerStopWithoutStart(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	m.Inc(CounterSecretsCreated, 2)
	// Manually drain events since loop not running
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			goto drained
		}
	}
drained:
	m.Stop(ctx) // should flush without panic
	row := db.QueryRowContext(ctx, `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsCreated)
	var v int64
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v != 2 {
		t.Fatalf("expected 2 got %d", v)
	}
}

func TestManagerChannelFullDrop(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	// Replace events channel with very small buffer to force drops.
	m.events = make(chan event, 1)
	// fill channel with one inc we won't drain yet.
	m.Inc(CounterSecretsCreated, 1)
	// This second very large inc should be dropped due to full channel.
	m.Inc(CounterSecretsCreated, 100)
	// Drain existing single event.
	ev := <-m.events
	m.apply(ev)
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	row := db.QueryRowContext(ctx, `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsCreated)
	var v int64
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v != 1 {
		t.Fatalf("expected only first event persisted got %d", v)
	}
}

func TestManagerLoopContextCancel(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{FlushInterval: 15 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	m.Start(ctx)
	m.Inc(CounterSecretsCreated, 3)
	time.Sleep(30 * time.Millisecond) // allow at least one flush cycle
	cancel()                          // triggers context_cancel path
	// give loop time to exit
	time.Sleep(10 * time.Millisecond)
	m.Stop(context.Background())
	row := db.QueryRowContext(context.Background(), `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsCreated)
	var v int64
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v == 0 {
		t.Fatalf("expected flushed value after loop context cancel")
	}
}

func TestManagerIncNegativeIgnored(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	m.Inc(CounterSecretsCreated, -5) // should be ignored
	// drain (should be nothing)
	select {
	case ev := <-m.events:
		t.Fatalf("unexpected event %+v", ev)
	default:
	}
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// ensure no row created
	rows, err := db.QueryContext(ctx, `SELECT value FROM metrics_counters WHERE name=?`, CounterSecretsCreated)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if rows.Next() {
		t.Fatalf("expected no row for ignored negative inc")
	}
}

func TestManagerObserveChannelFullDrop(t *testing.T) {
	db := openTempDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}
	m.events = make(chan event, 1)
	m.Observe(SummaryJanitorDeletedPerCycle, 10) // fills buffer
	m.Observe(SummaryJanitorDeletedPerCycle, 20) // dropped
	ev := <-m.events
	m.apply(ev)
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	_, summaries, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	agg, ok := summaries[SummaryJanitorDeletedPerCycle]
	if !ok {
		t.Fatalf("missing summary after apply")
	}
	if agg.count != 1 || agg.sum != 10 {
		t.Fatalf("expected only first observe kept %+v", agg)
	}
}
