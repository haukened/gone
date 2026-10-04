package metrics

import (
	"context"
	"testing"
	"time"
)

func TestManagerIncFlush(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	initMetricsSchema(ctx, t, m)

	m.Inc(CounterSecretsCreated, 1)
	m.Inc(CounterSecretsCreated, 2)
	flushMetricsTestEvents(ctx, t, m)
	if got := readMetricsTestCounter(ctx, t, db, CounterSecretsCreated); got != 3 {
		t.Fatalf("expected 3 got %d", got)
	}
}

func TestManagerSnapshotMergesDeltas(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	if _, err := db.ExecContext(ctx, `INSERT INTO metrics_counters(name,value) VALUES(?,10)`, CounterSecretsCreated); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m.Inc(CounterSecretsCreated, 5)
	drainMetricsTestEvents(m)
	cnt, _, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if cnt[CounterSecretsCreated] != 15 {
		t.Fatalf("expected merged 15 got %d", cnt[CounterSecretsCreated])
	}
}

func TestManagerChannelFullDrop(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	m.events = make(chan event, 1)
	m.Inc(CounterSecretsCreated, 1)
	m.Inc(CounterSecretsCreated, 100)
	m.apply(<-m.events)
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := readMetricsTestCounter(ctx, t, db, CounterSecretsCreated); got != 1 {
		t.Fatalf("expected only first event persisted got %d", got)
	}
}

func TestManagerIncNegativeIgnored(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	m.Inc(CounterSecretsCreated, -5)
	select {
	case ev := <-m.events:
		t.Fatalf("unexpected event %+v", ev)
	default:
	}
	flushMetricsTestEvents(ctx, t, m)
	assertMetricsTestNoCounter(ctx, t, db, CounterSecretsCreated)
}
