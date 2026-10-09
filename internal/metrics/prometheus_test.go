package metrics

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestWriteSummariesGolden(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]SummaryAgg
		want string
	}{
		{
			name: "recorded",
			in:   map[string]SummaryAgg{SummaryJanitorDeletedPerCycle: {Count: 2, Sum: 5, Min: 1, Max: 4}},
			want: summaryGolden("5", "2", "1", "4"),
		},
		{name: "zero filled", in: map[string]SummaryAgg{}, want: summaryGolden("0", "0", "0", "0")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			writeSummaries(&b, tc.in)
			if got := b.String(); got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestWriteSummariesUnknownName(t *testing.T) {
	var b bytes.Buffer
	writeSummaries(&b, map[string]SummaryAgg{"custom": {Count: 1, Sum: 3, Min: 3, Max: 3}})
	for _, line := range []string{"# HELP gone_custom Summary custom.\n", "gone_custom_sum 3\n", "gone_janitor_deleted_per_cycle_count 0\n"} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("missing %q in:\n%s", line, b.String())
		}
	}
}

// summaryGolden is the expected text for the janitor summary.
func summaryGolden(sum, count, minimum, maximum string) string {
	const n = "gone_janitor_deleted_per_cycle"
	return "# HELP " + n + " Secrets deleted per janitor run.\n" +
		"# TYPE " + n + " summary\n" +
		n + "_sum " + sum + "\n" +
		n + "_count " + count + "\n" +
		"# HELP " + n + "_min Smallest observation of " + n + ".\n" +
		"# TYPE " + n + "_min gauge\n" +
		n + "_min " + minimum + "\n" +
		"# HELP " + n + "_max Largest observation of " + n + ".\n" +
		"# TYPE " + n + "_max gauge\n" +
		n + "_max " + maximum + "\n"
}

func TestWriteMetricsGolden(t *testing.T) {
	ms := []Metric{
		{Name: "z_total", Help: "Last.", Type: "counter", Value: 7},
		{Name: "a_info", Help: "Line one\nback\\slash.", Labels: []Label{{"v", "q\"b\\s\nn"}, {"bad-label", "x"}}, Value: 1},
		{Name: "a_info", Labels: []Label{{"v", "two"}}, Value: 0.5},
		{Name: "bad name", Help: "Skipped.", Value: 1},
	}
	var b bytes.Buffer
	writeMetrics(&b, ms)
	want := "# HELP a_info Line one\\nback\\\\slash.\n" +
		"# TYPE a_info gauge\n" +
		"a_info{v=\"q\\\"b\\\\s\\nn\"} 1\n" +
		"a_info{v=\"two\"} 0.5\n" +
		"# HELP z_total Last.\n" +
		"# TYPE z_total counter\n" +
		"z_total 7\n"
	if got := b.String(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteCounters(t *testing.T) {
	var b bytes.Buffer
	writeCounters(&b, map[string]int64{CounterSecretsCreated: 3, "custom_total": 2, "bad name": 1})
	out := b.String()
	for _, line := range []string{
		"gone_secrets_created_total 3\n",
		"# HELP gone_custom_total Counter custom_total.\n",
		"gone_custom_total 2\n",
		"gone_requests_expired_total 0\n",
		"# TYPE gone_secrets_claims_expired_total counter\n",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("missing %q in:\n%s", line, out)
		}
	}
	if strings.Contains(out, "bad name") {
		t.Errorf("invalid name written:\n%s", out)
	}
	if got, want := strings.Count(out, "# TYPE "), len(counterHelp)+1; got != want {
		t.Errorf("TYPE lines = %d, want %d", got, want)
	}
	if strings.Index(out, "gone_custom_total") > strings.Index(out, "gone_rate_limited_create_total") {
		t.Errorf("counters not sorted:\n%s", out)
	}
}

// sampleLine, helpLine and typeLine match the text format's three line kinds.
var (
	helpLine   = regexp.MustCompile(`^# HELP [a-zA-Z_:][a-zA-Z0-9_:]* \S.*$`)
	typeLine   = regexp.MustCompile(`^# TYPE [a-zA-Z_:][a-zA-Z0-9_:]* (counter|gauge|summary)$`)
	sampleLine = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*"(,[a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*")*\})? -?[0-9.e+]+$`)
)

func TestPrometheusOutputGrammar(t *testing.T) {
	f := &fakeSnapshot{c: map[string]int64{CounterSecretsCreated: 3}, s: map[string]SummaryAgg{}}
	h := PrometheusHandler(f, BuildInfoSource("v1.2.3"), RuntimeSource(testStart), nil)
	rw := serve(t, h, http.StatusOK)
	if ct := rw.Header().Get("Content-Type"); ct != ContentType {
		t.Fatalf("Content-Type = %q", ct)
	}
	if got := rw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	body := rw.Body.String()
	if !strings.HasSuffix(body, "\n") {
		t.Fatal("body must end with a newline")
	}
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if !helpLine.MatchString(line) && !typeLine.MatchString(line) && !sampleLine.MatchString(line) {
			t.Errorf("malformed line %q", line)
		}
	}
	if !strings.Contains(body, `gone_build_info{version="v1.2.3",goversion="go`) {
		t.Errorf("missing build info:\n%s", body)
	}
}

func TestPrometheusHandlerErrors(t *testing.T) {
	failing := func(context.Context) ([]Metric, error) { return nil, errors.New("db down") }
	cases := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"snapshot", PrometheusHandler(&fakeSnapshot{err: errors.New("snapshot failed")})},
		{"source", PrometheusHandler(&fakeSnapshot{}, BuildInfoSource("v1"), failing)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rw := serve(t, tc.h, http.StatusInternalServerError)
			if rw.Body.Len() != 0 {
				t.Fatalf("partial body on error: %q", rw.Body.String())
			}
		})
	}
}

// serve runs a GET /metrics through h and verifies the status.
func serve(t *testing.T, h http.HandlerFunc, want int) *httptest.ResponseRecorder {
	t.Helper()
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rw.Code != want {
		t.Fatalf("status = %d, want %d", rw.Code, want)
	}
	return rw
}
