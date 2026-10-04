package metrics

import (
	"context"
	"database/sql"
	"errors"
)

// flush writes in-memory deltas to SQLite in a single transaction and resets them.
func (m *Manager) flush(ctx context.Context) error {
	cCopy, sCopy, ok := m.swapAndCopyDeltas()
	if !ok { // nothing to flush
		return nil
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := m.upsertCounters(ctx, tx, cCopy); err != nil {
		return err
	}
	if err := m.upsertSummaries(ctx, tx, sCopy); err != nil {
		return err
	}
	return tx.Commit()
}

// swapAndCopyDeltas copies in-memory deltas and resets maps under lock.
// Returns false if there is nothing to flush.
func (m *Manager) swapAndCopyDeltas() (map[string]int64, map[string]*SummaryAgg, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.counters) == 0 && len(m.summaries) == 0 {
		return nil, nil, false
	}
	cCopy := make(map[string]int64, len(m.counters))
	for k, v := range m.counters {
		cCopy[k] = v
	}
	sCopy := make(map[string]*SummaryAgg, len(m.summaries))
	for k, v := range m.summaries {
		cp := *v
		sCopy[k] = &cp
	}
	m.counters = make(map[string]int64)
	m.summaries = make(map[string]*SummaryAgg)
	return cCopy, sCopy, true
}

// upsertCounters persists counter deltas.
func (m *Manager) upsertCounters(ctx context.Context, tx *sql.Tx, counters map[string]int64) error {
	for name, delta := range counters {
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_counters(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value = value + excluded.value`, name, delta); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return errors.Join(err, rbErr)
			}
			return err
		}
	}
	return nil
}

// upsertSummaries persists summary aggregates.
func (m *Manager) upsertSummaries(ctx context.Context, tx *sql.Tx, sums map[string]*SummaryAgg) error {
	for name, agg := range sums {
		if _, err := tx.ExecContext(ctx, `INSERT INTO metrics_summaries(name,count,sum,min,max) VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET count = metrics_summaries.count + excluded.count, sum = metrics_summaries.sum + excluded.sum, min = MIN(metrics_summaries.min, excluded.min), max = MAX(metrics_summaries.max, excluded.max)`, name, agg.Count, agg.Sum, agg.Min, agg.Max); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return errors.Join(err, rbErr)
			}
			return err
		}
	}
	return nil
}
