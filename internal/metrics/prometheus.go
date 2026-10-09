package metrics

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ContentType is the media type of the Prometheus text exposition format.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// namespace prefixes every application counter and summary on /metrics.
const namespace = "gone_"

// Metric is one sample a Source adds to /metrics. Type is "gauge" (the
// default when empty) or "counter".
type Metric struct {
	Name   string
	Help   string
	Type   string
	Labels []Label
	Value  float64
}

// Label is one name="value" pair on a Metric.
type Label struct {
	Name  string
	Value string
}

// Source supplies metrics computed at scrape time, such as storage counts.
type Source func(ctx context.Context) ([]Metric, error)

// counterHelp describes each counter the application records. Known counters
// are written as 0 before their first increment, so a scrape always shows
// the full set.
var counterHelp = map[string]string{
	CounterSecretsCreated:       "Secrets stored.",
	CounterSecretsConsumed:      "Secrets opened and deleted.",
	CounterSecretsRevoked:       "Secrets the sender deleted with the manage link before they were opened.",
	CounterSecretsExpiredDelete: "Secrets the janitor removed: expired ones and opened ones whose claim lease lapsed.",
	CounterClaimsExpired:        "Opened secrets deleted because receipt was never confirmed within the claim lease.",
	CounterRateLimitedCreate:    "Create requests rejected with 429.",
	CounterRateLimitedRead:      "Read, acknowledge, status, and revoke requests rejected with 429.",
	CounterRequestsCreated:      "Secret requests opened.",
	CounterRequestsFilled:       "Secret requests answered.",
	CounterRequestsOpened:       "Request replies opened and deleted.",
	CounterRequestsCancelled:    "Requests or unopened replies the requester cancelled.",
	CounterRequestsExpired:      "Requests the janitor removed because nobody answered in time.",
}

// summaryHelp describes each summary the application records.
var summaryHelp = map[string]string{
	SummaryJanitorDeletedPerCycle: "Secrets deleted per janitor run.",
}

// validName matches a legal Prometheus metric or label name.
var validName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

// helpEscaper and labelEscaper apply the text format's escaping rules.
var (
	helpEscaper  = strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	labelEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
)

// PrometheusHandler returns an http.HandlerFunc that writes the snapshot and
// every source's metrics in the Prometheus text format. Any failure yields a
// 500 with no body, so a scrape never sees partial data. It does not check
// authorization; NewMux wraps it.
//
// Parameters:
//   - provider: persisted counters and summaries.
//   - sources: metrics computed at scrape time, written after the snapshot.
//
// Returns:
//   - http.HandlerFunc: the /metrics handler.
func PrometheusHandler(provider SnapshotProvider, sources ...Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		if err := gather(r.Context(), &b, provider, sources); err != nil {
			slog.Default().Error("metrics scrape", "domain", "metrics", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", ContentType)
		_, _ = w.Write(b.Bytes())
	}
}

// gather writes the snapshot, then each source, to b.
//
// Parameters:
//   - ctx: request context.
//   - b: output buffer.
//   - provider: persisted counters and summaries.
//   - sources: metrics computed at scrape time; nil entries are skipped.
//
// Returns the first snapshot or source error.
func gather(ctx context.Context, b *bytes.Buffer, provider SnapshotProvider, sources []Source) error {
	counters, summaries, err := provider.Snapshot(ctx)
	if err != nil {
		return err
	}
	writeCounters(b, counters)
	writeSummaries(b, summaries)
	var extra []Metric
	for _, src := range sources {
		if src == nil {
			continue
		}
		ms, err := src(ctx)
		if err != nil {
			return err
		}
		extra = append(extra, ms...)
	}
	writeMetrics(b, extra)
	return nil
}

// writeCounters writes every known counter (0 when unrecorded) plus any
// other recorded counter, sorted by name.
//
// Parameters:
//   - b: output buffer.
//   - counters: recorded counter values by unprefixed name.
func writeCounters(b *bytes.Buffer, counters map[string]int64) {
	for _, n := range knownAndRecorded(counterHelp, counters) {
		help := counterHelp[n]
		if help == "" {
			help = "Counter " + n + "."
		}
		writeFamily(b, namespace+n, help, "counter")
		writeSample(b, namespace+n, nil, float64(counters[n]))
	}
}

// writeSummaries writes every known summary (zero when unrecorded) plus any
// other recorded summary as a quantile-free Prometheus summary (_sum and
// _count), with its lifetime minimum and maximum as gauges.
//
// Parameters:
//   - b: output buffer.
//   - summaries: recorded aggregates by unprefixed name.
func writeSummaries(b *bytes.Buffer, summaries map[string]SummaryAgg) {
	for _, n := range knownAndRecorded(summaryHelp, summaries) {
		agg, name := summaries[n], namespace+n
		help := summaryHelp[n]
		if help == "" {
			help = "Summary " + n + "."
		}
		writeFamily(b, name, help, "summary")
		writeSample(b, name+"_sum", nil, float64(agg.Sum))
		writeSample(b, name+"_count", nil, float64(agg.Count))
		writeFamily(b, name+"_min", "Smallest observation of "+name+".", "gauge")
		writeSample(b, name+"_min", nil, float64(agg.Min))
		writeFamily(b, name+"_max", "Largest observation of "+name+".", "gauge")
		writeSample(b, name+"_max", nil, float64(agg.Max))
	}
}

// writeMetrics writes source metrics grouped by name, in name order. HELP and
// TYPE come from the first sample of each name.
//
// Parameters:
//   - b: output buffer.
//   - ms: metrics from every source.
func writeMetrics(b *bytes.Buffer, ms []Metric) {
	slices.SortStableFunc(ms, func(x, y Metric) int { return strings.Compare(x.Name, y.Name) })
	for i, m := range ms {
		if i == 0 || ms[i-1].Name != m.Name {
			writeFamily(b, m.Name, m.Help, m.typeName())
		}
		writeSample(b, m.Name, m.Labels, m.Value)
	}
}

// typeName returns the metric's Prometheus type, defaulting to gauge.
func (m Metric) typeName() string {
	if m.Type == "" {
		return "gauge"
	}
	return m.Type
}

// writeFamily writes the HELP and TYPE lines for a metric family. Invalid
// names are skipped, along with their samples in writeSample.
//
// Parameters:
//   - b: output buffer.
//   - name: full metric name.
//   - help: one-line description.
//   - typ: counter, gauge, or summary.
func writeFamily(b *bytes.Buffer, name, help, typ string) {
	if !validName.MatchString(name) {
		return
	}
	b.WriteString("# HELP " + name + " " + helpEscaper.Replace(help) + "\n")
	b.WriteString("# TYPE " + name + " " + typ + "\n")
}

// writeSample writes one sample line, skipping invalid metric or label names.
//
// Parameters:
//   - b: output buffer.
//   - name: full metric name.
//   - labels: optional labels, written in the given order.
//   - v: sample value.
func writeSample(b *bytes.Buffer, name string, labels []Label, v float64) {
	if !validName.MatchString(name) {
		return
	}
	b.WriteString(name)
	if len(labels) > 0 {
		parts := make([]string, 0, len(labels))
		for _, l := range labels {
			if validName.MatchString(l.Name) {
				parts = append(parts, l.Name+`="`+labelEscaper.Replace(l.Value)+`"`)
			}
		}
		b.WriteString("{" + strings.Join(parts, ",") + "}")
	}
	b.WriteString(" " + strconv.FormatFloat(v, 'g', -1, 64) + "\n")
}

// knownAndRecorded returns every name in known or recorded, sorted.
//
// Parameters:
//   - known: help text by name for the metrics the application records.
//   - recorded: values by name from the snapshot.
//
// Returns:
//   - []string: the union of both key sets in ascending order.
func knownAndRecorded[V any](known map[string]string, recorded map[string]V) []string {
	names := make([]string, 0, len(known)+len(recorded))
	for n := range known {
		names = append(names, n)
	}
	for n := range recorded {
		if _, ok := known[n]; !ok {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}
