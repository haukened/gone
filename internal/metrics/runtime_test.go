package metrics

import (
	"context"
	"runtime"
	rtmetrics "runtime/metrics"
	"testing"
	"time"
)

// testStart is a fixed process start time for runtime source tests.
var testStart = time.Unix(1700000000, 500_000_000)

func TestRuntimeSource(t *testing.T) {
	ms, err := RuntimeSource(testStart)(context.Background())
	if err != nil {
		t.Fatalf("RuntimeSource: %v", err)
	}
	got := make(map[string]Metric, len(ms))
	for _, m := range ms {
		got[m.Name] = m
	}
	for _, name := range []string{"go_goroutines", "go_memstats_heap_alloc_bytes", "go_memstats_sys_bytes", "go_gc_cycles_total", "process_start_time_seconds"} {
		m, ok := got[name]
		if !ok {
			t.Fatalf("missing %s in %+v", name, ms)
		}
		if m.Value < 0 || m.Help == "" {
			t.Errorf("%s = %+v", name, m)
		}
	}
	if got["go_goroutines"].Value < 1 || got["go_memstats_sys_bytes"].Value <= 0 {
		t.Errorf("implausible runtime values: %+v", ms)
	}
	if got["go_gc_cycles_total"].typeName() != "counter" || got["go_goroutines"].typeName() != "gauge" {
		t.Errorf("wrong types: %+v", ms)
	}
	if v := got["process_start_time_seconds"].Value; v != 1700000000.5 {
		t.Errorf("process_start_time_seconds = %v", v)
	}
}

func TestSampleValueKinds(t *testing.T) {
	s := []rtmetrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}, {Name: "/no/such:metric"}}
	rtmetrics.Read(s)
	if s[0].Value.Kind() != rtmetrics.KindFloat64 || sampleValue(s[0].Value) < 0 {
		t.Errorf("float sample = %v", s[0].Value.Kind())
	}
	if got := sampleValue(s[1].Value); got != 0 {
		t.Errorf("unsupported sample = %v, want 0", got)
	}
}

func TestBuildInfoSource(t *testing.T) {
	ms, err := BuildInfoSource("v3.9.0")(context.Background())
	if err != nil || len(ms) != 1 {
		t.Fatalf("BuildInfoSource = %+v, %v", ms, err)
	}
	m := ms[0]
	want := []Label{{"version", "v3.9.0"}, {"goversion", runtime.Version()}}
	if m.Name != "gone_build_info" || m.Value != 1 || len(m.Labels) != 2 || m.Labels[0] != want[0] || m.Labels[1] != want[1] {
		t.Fatalf("build info = %+v", m)
	}
}
