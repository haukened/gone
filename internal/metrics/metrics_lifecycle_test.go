package metrics

import (
	"context"
	"testing"
	"time"
)

func TestManagerStopFinalFlush(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	initMetricsSchema(ctx, t, m)
	m.Inc(CounterSecretsConsumed, 4)
	drainMetricsTestEvents(m)
	m.Stop(context.Background())
	cancel()
	if got := readMetricsTestCounter(context.Background(), t, db, CounterSecretsConsumed); got != 4 {
		t.Fatalf("expected 4 got %d", got)
	}
}

func TestManagerFlushEmpty(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush empty: %v", err)
	}
}

func TestManagerStartIdempotent(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	initMetricsSchema(ctx, t, m)
	m.Start(ctx)
	m.Start(ctx)
	m.Inc(CounterSecretsCreated, 1)
	m.Stop(context.Background())
	if got := readMetricsTestCounter(context.Background(), t, db, CounterSecretsCreated); got == 0 {
		t.Fatalf("expected counter increment persisted")
	}
}

func TestManagerStopWithoutStart(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	m.Inc(CounterSecretsCreated, 2)
	drainMetricsTestEvents(m)
	m.Stop(ctx)
	if got := readMetricsTestCounter(ctx, t, db, CounterSecretsCreated); got != 2 {
		t.Fatalf("expected 2 got %d", got)
	}
}

func TestManagerLoopContextCancel(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	initMetricsSchema(ctx, t, m)
	m.Inc(CounterSecretsCreated, 3)
	m.Start(ctx)
	cancel()
	<-m.done
	m.Stop(context.Background())
	if got := readMetricsTestCounter(context.Background(), t, db, CounterSecretsCreated); got != 3 {
		t.Fatalf("value = %d, want 3 after loop context cancel", got)
	}
}

// TestManagerStopUnstartedDrains checks that Stop on a never-started Manager
// persists events that were queued but not yet applied.
func TestManagerStopUnstartedDrains(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	m.Inc(CounterSecretsConsumed, 2)
	m.Stop(ctx)
	if got := readMetricsTestCounter(ctx, t, db, CounterSecretsConsumed); got != 2 {
		t.Fatalf("value = %d, want 2", got)
	}
}
