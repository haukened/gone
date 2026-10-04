package metrics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openMetricsTestDB creates an isolated sqlite database file for tests.
func openMetricsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "m.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return db
}

// initMetricsSchema initializes the metrics schema for tests.
func initMetricsSchema(ctx context.Context, t *testing.T, m *Manager) {
	t.Helper()
	if err := m.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
}

// drainMetricsTestEvents applies all queued events to the manager state.
func drainMetricsTestEvents(m *Manager) {
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			return
		}
	}
}

// flushMetricsTestEvents drains queued events and flushes manager state.
func flushMetricsTestEvents(ctx context.Context, t *testing.T, m *Manager) {
	t.Helper()
	drainMetricsTestEvents(m)
	if err := m.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

// readMetricsTestCounter reads one persisted counter value.
func readMetricsTestCounter(ctx context.Context, t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	row := db.QueryRowContext(ctx, `SELECT value FROM metrics_counters WHERE name=?`, name)
	var v int64
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return v
}

// assertMetricsTestNoCounter verifies that a counter row is absent.
func assertMetricsTestNoCounter(ctx context.Context, t *testing.T, db *sql.DB, name string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT value FROM metrics_counters WHERE name=?`, name)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	}()
	if rows.Next() {
		t.Fatalf("expected no row for %s", name)
	}
}

// assertMetricsTestSummary verifies summary aggregate values.
func assertMetricsTestSummary(t *testing.T, agg SummaryAgg, count, sum, minValue, maxValue int64) {
	t.Helper()
	if agg.Count != count || agg.Sum != sum || agg.Min != minValue || agg.Max != maxValue {
		t.Fatalf("summary = %+v, want count=%d sum=%d min=%d max=%d", agg, count, sum, minValue, maxValue)
	}
}
