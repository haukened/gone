package metrics

import (
	"context"
	"runtime"
	rtmetrics "runtime/metrics"
	"time"
)

// runtimeGauges maps runtime/metrics samples to the names client_golang
// uses, so existing Go dashboards work unchanged.
var runtimeGauges = []struct {
	sample, name, help, typ string
}{
	{"/sched/goroutines:goroutines", "go_goroutines", "Number of goroutines that currently exist.", "gauge"},
	{"/memory/classes/heap/objects:bytes", "go_memstats_heap_alloc_bytes", "Number of heap bytes allocated and still in use.", "gauge"},
	{"/memory/classes/total:bytes", "go_memstats_sys_bytes", "Number of bytes obtained from the system.", "gauge"},
	{"/gc/cycles/total:gc-cycles", "go_gc_cycles_total", "Number of completed GC cycles.", "counter"},
}

// RuntimeSource reports goroutines, memory, GC cycles, and the process start
// time. Reading runtime/metrics does not stop the world.
//
// Parameters:
//   - start: when the process started.
//
// Returns:
//   - Source: the runtime metrics source; it never fails.
func RuntimeSource(start time.Time) Source {
	return func(context.Context) ([]Metric, error) {
		samples := make([]rtmetrics.Sample, len(runtimeGauges))
		for i, g := range runtimeGauges {
			samples[i].Name = g.sample
		}
		rtmetrics.Read(samples)
		out := make([]Metric, 0, len(samples)+1)
		for i, g := range runtimeGauges {
			out = append(out, Metric{Name: g.name, Help: g.help, Type: g.typ, Value: sampleValue(samples[i].Value)})
		}
		out = append(out, Metric{
			Name:  "process_start_time_seconds",
			Help:  "Start time of the process since unix epoch in seconds.",
			Value: float64(start.UnixNano()) / 1e9,
		})
		return out, nil
	}
}

// sampleValue converts a runtime/metrics value to a float; unsupported kinds
// (a metric this Go version lacks) read as 0.
func sampleValue(v rtmetrics.Value) float64 {
	switch v.Kind() {
	case rtmetrics.KindUint64:
		return float64(v.Uint64())
	case rtmetrics.KindFloat64:
		return v.Float64()
	default:
		return 0
	}
}

// BuildInfoSource reports gone_build_info, always 1, labeled with the
// release and the Go version it was built with.
//
// Parameters:
//   - version: the running release, e.g. "v3.9.0" or "dev".
//
// Returns:
//   - Source: the build info source; it never fails.
func BuildInfoSource(version string) Source {
	m := Metric{
		Name:   namespace + "build_info",
		Help:   "Gone build information; the value is always 1.",
		Labels: []Label{{"version", version}, {"goversion", runtime.Version()}},
		Value:  1,
	}
	return func(context.Context) ([]Metric, error) {
		return []Metric{m}, nil
	}
}
