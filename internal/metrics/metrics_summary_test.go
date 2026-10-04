package metrics

import (
	"context"
	"testing"
	"time"
)

func TestManagerObserveFlushSnapshot(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: 500 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	initMetricsSchema(ctx, t, m)

	m.Observe(SummaryJanitorDeletedPerCycle, 5)
	m.Observe(SummaryJanitorDeletedPerCycle, 7)
	flushMetricsTestEvents(ctx, t, m)
	counters, summaries, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(counters) != 0 {
		t.Fatalf("unexpected counters %+v", counters)
	}
	assertMetricsTestSummary(t, summaries[SummaryJanitorDeletedPerCycle], 2, 12, 5, 7)
}

func TestManagerSummaryLayering(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{FlushInterval: time.Hour})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	if _, err := db.ExecContext(ctx, `INSERT INTO metrics_summaries(name,count,sum,min,max) VALUES(?,?,?,?,?)`, SummaryJanitorDeletedPerCycle, 3, 30, 5, 20); err != nil {
		t.Fatalf("seed summary: %v", err)
	}
	m.Observe(SummaryJanitorDeletedPerCycle, 4)
	m.Observe(SummaryJanitorDeletedPerCycle, 25)
	m.Observe(SummaryJanitorDeletedPerCycle, 6)
	drainMetricsTestEvents(m)
	counters, summaries, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(counters) != 0 {
		t.Fatalf("unexpected counters %+v", counters)
	}
	assertMetricsTestSummary(t, summaries[SummaryJanitorDeletedPerCycle], 6, 65, 4, 25)
}

func TestManagerObserveChannelFullDrop(t *testing.T) {
	db := openMetricsTestDB(t)
	m := New(db, Config{})
	ctx := context.Background()
	initMetricsSchema(ctx, t, m)
	m.events = make(chan event, 1)
	m.Observe(SummaryJanitorDeletedPerCycle, 10)
	m.Observe(SummaryJanitorDeletedPerCycle, 20)
	m.apply(<-m.events)
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	_, summaries, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	assertMetricsTestSummary(t, summaries[SummaryJanitorDeletedPerCycle], 1, 10, 10, 10)
}
