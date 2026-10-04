package metrics

import (
	"context"
	"database/sql"
	"errors"
)

// Snapshot returns current (persisted + in-memory deltas) by reading persisted
// state and layering deltas. This is optional and may be refined later.
func (m *Manager) Snapshot(ctx context.Context) (counters map[string]int64, summaries map[string]SummaryAgg, err error) {
	counters, err = m.loadPersistedCounters(ctx)
	if err != nil {
		return nil, nil, err
	}
	summaries, err = m.loadPersistedSummaries(ctx)
	if err != nil {
		return nil, nil, err
	}
	m.layerDeltas(counters, summaries)
	return counters, summaries, nil
}

// loadPersistedCounters reads counters from storage.
func (m *Manager) loadPersistedCounters(ctx context.Context) (counters map[string]int64, err error) {
	counters = make(map[string]int64)
	rows, err := m.db.QueryContext(ctx, `SELECT name, value FROM metrics_counters`)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var n string
		var v int64
		if err := rows.Scan(&n, &v); err != nil {
			return nil, err
		}
		counters[n] = v
	}
	return counters, rows.Err()
}

// loadPersistedSummaries reads summaries from storage.
func (m *Manager) loadPersistedSummaries(ctx context.Context) (summaries map[string]SummaryAgg, err error) {
	summaries = make(map[string]SummaryAgg)
	rows, err := m.db.QueryContext(ctx, `SELECT name, count, sum, min, max FROM metrics_summaries`)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var n string
		var c, s, mn, mx int64
		if err := rows.Scan(&n, &c, &s, &mn, &mx); err != nil {
			return nil, err
		}
		summaries[n] = SummaryAgg{Count: c, Sum: s, Min: mn, Max: mx}
	}
	return summaries, rows.Err()
}

// layerDeltas merges in-memory deltas onto persisted values.
func (m *Manager) layerDeltas(counters map[string]int64, summaries map[string]SummaryAgg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for n, v := range m.counters {
		counters[n] += v
	}
	for n, agg := range m.summaries {
		layerSummaryDelta(summaries, n, agg)
	}
}

// layerSummaryDelta merges agg into summaries under name.
//
// Parameters:
//   - summaries: summaries being layered.
//   - name: summary metric name.
//   - agg: in-memory aggregate to merge.
func layerSummaryDelta(summaries map[string]SummaryAgg, name string, agg *SummaryAgg) {
	cur := summaries[name]
	if cur.Count == 0 { // no persisted value yet
		summaries[name] = *agg
		return
	}
	cur.Count += agg.Count
	cur.Sum += agg.Sum
	if agg.Min < cur.Min {
		cur.Min = agg.Min
	}
	if agg.Max > cur.Max {
		cur.Max = agg.Max
	}
	summaries[name] = cur
}

// sqlRows is the row iterator subset returned by database/sql queries.
type sqlRows interface {
	Close() error
	Err() error
	Next() bool
	Scan(...any) error
}

var _ sqlRows = (*sql.Rows)(nil)
