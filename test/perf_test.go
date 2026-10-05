package integration_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Performance target from the contributor instructions: p95 latency under
// 50ms at 100 requests per second.
const (
	perfTargetRPS = 100
	perfTargetP95 = 50 * time.Millisecond
	// perfFlowRequests is the number of requests in one create, claim,
	// acknowledge flow.
	perfFlowRequests = 3
	// perfSmallBytes is stored inline in SQLite; perfLargeBytes exceeds the
	// default 8 KiB inline limit and goes to a filesystem blob.
	perfSmallBytes = 1 << 10
	perfLargeBytes = 64 << 10
	// perfLargeEvery makes every Nth flow use a blob-sized secret.
	perfLargeEvery = 10
)

// perfRecorder collects request latencies per endpoint and counts failures.
type perfRecorder struct {
	mu        sync.Mutex
	latencies map[string][]time.Duration
	failures  []string
}

// record stores one latency for an endpoint.
//
// Parameters:
//   - endpoint: label such as "create".
//   - d: the request's latency.
func (r *perfRecorder) record(endpoint string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latencies[endpoint] = append(r.latencies[endpoint], d)
}

// fail stores a failed flow. Failures are reported on the test goroutine,
// since t.Fatal must not be called from flow goroutines.
//
// Parameters:
//   - err: what went wrong.
func (r *perfRecorder) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = append(r.failures, err.Error())
}

// TestPerfLatencyTarget drives the real HTTP stack (SQLite index and blob
// directory under the test's temp dir, FULL synchronous) at the target rate
// and checks p95 latency.
//
// It is open-loop: flows start on a fixed schedule whether or not earlier ones
// have finished, so a slow server cannot lower the offered rate and hide its
// own latency. Each flow is create, claim, acknowledge, and every tenth uses a
// blob-sized secret. Latency is measured per request, from send to the last
// body byte.
//
// Timing depends on the machine, so the test only runs when GONE_PERF=1
// (`task perf`). GONE_PERF_DURATION overrides the 10s run length, and
// GONE_PERF_RPS offers a different rate to find a machine's limit (the p95
// target is still checked, but the target rate is 100). The data
// directory follows TMPDIR, so point TMPDIR at the disk you deploy on to
// measure that disk. CI points it at RAM (/dev/shm) so shared-runner disk
// stalls don't fail the build; there the test guards the code path.
//
// Parameters:
//   - t: the test.
func TestPerfLatencyTarget(t *testing.T) {
	if os.Getenv("GONE_PERF") != "1" {
		t.Skip("set GONE_PERF=1 to run the latency target test (task perf)")
	}
	duration := 10 * time.Second
	if v := os.Getenv("GONE_PERF_DURATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			t.Fatalf("GONE_PERF_DURATION=%q: want a positive Go duration", v)
		}
		duration = d
	}

	rps := perfTargetRPS
	if v := os.Getenv("GONE_PERF_RPS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("GONE_PERF_RPS=%q: want a positive integer", v)
		}
		rps = n
	}

	srv := newServer(t, &fakeClock{t: time.Now()}, limits{})
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: rps},
	}
	t.Cleanup(client.CloseIdleConnections)
	rec := &perfRecorder{latencies: map[string][]time.Duration{}}

	// Warm up connections, the page cache and SQLite before measuring.
	for i := range 5 {
		if err := perfFlow(client, srv.URL, perfSmallBytes, nil); err != nil {
			t.Fatalf("warm-up flow %d: %v", i, err)
		}
	}

	interval := time.Second * perfFlowRequests / time.Duration(rps)
	flows := int(duration / interval)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range flows {
		time.Sleep(time.Until(start.Add(time.Duration(i) * interval)))
		size := perfSmallBytes
		if i%perfLargeEvery == 0 {
			size = perfLargeBytes
		}
		wg.Go(func() {
			if err := perfFlow(client, srv.URL, size, rec); err != nil {
				rec.fail(err)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	if len(rec.failures) > 0 {
		t.Fatalf("%d of %d flows failed; first: %s", len(rec.failures), flows, rec.failures[0])
	}
	var all []time.Duration
	for _, ep := range []string{"create", "claim", "ack"} {
		ds := rec.latencies[ep]
		all = append(all, ds...)
		t.Logf("%-7s n=%-5d %s", ep, len(ds), perfSummary(ds))
	}
	t.Logf("%-7s n=%-5d %s", "all", len(all), perfSummary(all))
	t.Logf("offered %.1f req/s over %s", float64(len(all))/elapsed.Seconds(), elapsed.Round(time.Millisecond))

	if p95 := percentile(all, 95); p95 > perfTargetP95 {
		t.Errorf("p95 latency %s exceeds the %s target at %d req/s", p95, perfTargetP95, rps)
	}
}

// perfFlow creates a secret of size bytes, claims it, and acknowledges it,
// recording each request's latency when rec is non-nil.
//
// Parameters:
//   - client: HTTP client to use.
//   - base: server base URL.
//   - size: ciphertext size in bytes.
//   - rec: latency recorder, or nil during warm-up.
//
// Returns:
//   - error: the first failed step, or nil.
func perfFlow(client *http.Client, base string, size int, rec *perfRecorder) error {
	body := make([]byte, size)
	nonce := make([]byte, 12)
	_, _ = rand.Read(body)
	_, _ = rand.Read(nonce)

	create, _ := http.NewRequest(http.MethodPost, base+"/api/secret", bytes.NewReader(body))
	create.Header = protocolHeaders(base64.RawURLEncoding.EncodeToString(nonce))
	var created struct {
		ID string `json:"id"`
	}
	resp, err := perfDo(client, create, "create", http.StatusCreated, rec)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(resp.body, &created); err != nil {
		return fmt.Errorf("create: decode: %w", err)
	}

	claim, _ := http.NewRequest(http.MethodGet, base+"/api/secret/"+created.ID, nil)
	resp, err = perfDo(client, claim, "claim", http.StatusOK, rec)
	if err != nil {
		return err
	}
	if len(resp.body) != size {
		return fmt.Errorf("claim: got %d bytes, want %d", len(resp.body), size)
	}

	ack, _ := http.NewRequest(http.MethodDelete, base+"/api/secret/"+created.ID, nil)
	ack.Header.Set("X-Gone-Claim", resp.header.Get("X-Gone-Claim"))
	_, err = perfDo(client, ack, "ack", http.StatusNoContent, rec)
	return err
}

// perfResponse is a fully read response.
type perfResponse struct {
	header http.Header
	body   []byte
}

// perfDo sends req, reads the whole body, checks the status, and records the
// latency under endpoint when rec is non-nil.
//
// Parameters:
//   - client: HTTP client to use.
//   - req: the request.
//   - endpoint: label for the recorder and errors.
//   - want: expected status code.
//   - rec: latency recorder, or nil.
//
// Returns:
//   - perfResponse: headers and body.
//   - error: transport, read, or status failure.
func perfDo(client *http.Client, req *http.Request, endpoint string, want int, rec *perfRecorder) (perfResponse, error) {
	began := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return perfResponse{}, fmt.Errorf("%s: %w", endpoint, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	elapsed := time.Since(began)
	if err != nil {
		return perfResponse{}, fmt.Errorf("%s: read body: %w", endpoint, err)
	}
	if resp.StatusCode != want {
		return perfResponse{}, fmt.Errorf("%s: status %d, want %d: %s", endpoint, resp.StatusCode, want, body)
	}
	if rec != nil {
		rec.record(endpoint, elapsed)
	}
	return perfResponse{header: resp.Header, body: body}, nil
}

// percentile returns the p-th percentile of ds by nearest rank.
//
// Parameters:
//   - ds: latencies; not modified.
//   - p: percentile in (0, 100].
//
// Returns:
//   - time.Duration: the percentile, or 0 for no samples.
func percentile(ds []time.Duration, p int) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := slices.Clone(ds)
	slices.Sort(s)
	rank := (p*len(s) + 99) / 100
	return s[max(rank-1, 0)]
}

// perfSummary formats p50, p95, p99, and max latencies.
//
// Parameters:
//   - ds: latencies.
//
// Returns:
//   - string: one summary line.
func perfSummary(ds []time.Duration) string {
	return fmt.Sprintf("p50=%-9s p95=%-9s p99=%-9s max=%s",
		percentile(ds, 50).Round(10*time.Microsecond), percentile(ds, 95).Round(10*time.Microsecond),
		percentile(ds, 99).Round(10*time.Microsecond), slices.Max(ds).Round(10*time.Microsecond))
}
