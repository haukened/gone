package metrics

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// loop runs the background event and flush dispatcher until stopped.
//
// Parameters:
//   - ctx: cancellation context for the dispatcher.
func (m *Manager) loop(ctx context.Context) {
	log := m.cfg.Logger.With("domain", "metrics")
	ticker := time.NewTicker(m.cfg.FlushInterval)
	defer func() {
		ticker.Stop()
		m.drain()
		close(m.done)
	}()
	for {
		if m.handleLoopEvent(ctx, ticker.C, log) {
			return
		}
	}
}

// handleLoopEvent applies one event, flush tick, or stop signal.
//
// Parameters:
//   - ctx: cancellation context.
//   - ticks: ticker channel for scheduled flushes.
//   - log: metrics logger.
//
// Returns true when the loop should stop.
func (m *Manager) handleLoopEvent(ctx context.Context, ticks <-chan time.Time, log *slog.Logger) bool {
	select {
	case <-ctx.Done():
		log.Info("metrics stop", "reason", "context_cancel")
		return true
	case <-m.stop:
		log.Info("metrics stop", "reason", "stop_signal")
		return true
	case ev := <-m.events:
		m.apply(ev)
		return false
	case <-ticks:
		m.flushLoopTick(ctx, log)
		return false
	}
}

// flushLoopTick persists queued metric deltas and logs non-cancellation errors.
//
// Parameters:
//   - ctx: request context for database work.
//   - log: metrics logger.
func (m *Manager) flushLoopTick(ctx context.Context, log *slog.Logger) {
	if err := m.flush(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("flush", "error", err)
	}
}

// drain applies every event already queued, without blocking, so that a final
// flush includes increments and observations recorded just before shutdown.
func (m *Manager) drain() {
	for {
		select {
		case ev := <-m.events:
			m.apply(ev)
		default:
			return
		}
	}
}

// apply merges one metric event into the in-memory deltas.
//
// Parameters:
//   - ev: metric event to apply.
func (m *Manager) apply(ev event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ev.kind {
	case eventInc:
		m.counters[ev.name] += ev.v
	case eventObserve:
		m.applySummaryObservation(ev.name, ev.v)
	}
}

// applySummaryObservation merges one summary observation into m.summaries.
//
// Parameters:
//   - name: summary metric name.
//   - value: observed value.
func (m *Manager) applySummaryObservation(name string, value int64) {
	agg := m.summaries[name]
	if agg == nil {
		m.summaries[name] = &SummaryAgg{Count: 1, Sum: value, Min: value, Max: value}
		return
	}
	agg.Count++
	agg.Sum += value
	if value < agg.Min {
		agg.Min = value
	}
	if value > agg.Max {
		agg.Max = value
	}
}
