package metrics

import (
	"context"

	"github.com/haukened/gone/internal/sqlrows"
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
func (m *Manager) loadPersistedCounters(ctx context.Context) (map[string]int64, error) {
	counters := make(map[string]int64)
	err := sqlrows.Query(ctx, m.db, `SELECT name, value FROM metrics_counters`, func(r sqlrows.Rows) error {
		var n string
		var v int64
		err := r.Scan(&n, &v)
		counters[n] = v
		return err
	})
	if err != nil {
		return nil, err
	}
	return counters, nil
}

// loadPersistedSummaries reads summaries from storage.
func (m *Manager) loadPersistedSummaries(ctx context.Context) (map[string]SummaryAgg, error) {
	summaries := make(map[string]SummaryAgg)
	err := sqlrows.Query(ctx, m.db, `SELECT name, count, sum, min, max FROM metrics_summaries`, func(r sqlrows.Rows) error {
		var n string
		var a SummaryAgg
		err := r.Scan(&n, &a.Count, &a.Sum, &a.Min, &a.Max)
		summaries[n] = a
		return err
	})
	if err != nil {
		return nil, err
	}
	return summaries, nil
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
