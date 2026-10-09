package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/metrics"
	"github.com/haukened/gone/v3/internal/store"
)

var metricDisabledCases = []struct {
	name  string
	addr  string
	token string
}{
	{"disabled", "", ""},
	{"address without token", "", ""},
}

// TestStartMetrics_DisabledAndTokenless covers no-op metrics configurations.
//
// Parameters:
//   - t: the test handle.
func TestStartMetrics_DisabledAndTokenless(t *testing.T) {
	cases := append([]struct {
		name  string
		addr  string
		token string
	}{}, metricDisabledCases...)
	cases[1].addr = freeAddr(t)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mgr := startMetricsForTest(t, tt.addr, tt.token)
			mgr.Stop(context.Background())
		})
	}
}

// TestStartMetrics_Enabled verifies the metrics endpoint starts with a token.
//
// Parameters:
//   - t: the test handle.
func TestStartMetrics_Enabled(t *testing.T) {
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, idx, _ := testStorage(t)
	mgr, err := startMetrics(ctx, db, &config.Config{MetricsAddr: addr, MetricsToken: "tok"},
		metrics.BuildInfoSource("v9.9.9"), storageSource(idx, realClock{}))
	if err != nil {
		t.Fatalf("startMetrics: %v", err)
	}
	defer mgr.Stop(context.Background())
	auth := http.Header{"Authorization": {"Bearer " + "tok"}}
	waitFor(t, "http://"+addr+"/", auth, http.StatusOK)
	body := getBody(t, "http://"+addr+"/metrics", auth)
	for _, want := range []string{`gone_build_info{version="v9.9.9"`, "gone_secrets_stored 0\n", "gone_requests_open 0\n", "gone_secrets_created_total 0\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing %q:\n%s", want, body)
		}
	}
}

// TestStorageSource covers the storage gauges, an index without Stats, and a
// query failure.
//
// Parameters:
//   - t: the test handle.
func TestStorageSource(t *testing.T) {
	if src := storageSource(struct{ store.Index }{}, realClock{}); src != nil {
		t.Fatal("index without Stats should yield a nil source")
	}
	db, idx, _ := testStorage(t)
	ms, err := storageSource(idx, realClock{})(context.Background())
	if err != nil || len(ms) != 3 {
		t.Fatalf("storage source = %+v, %v", ms, err)
	}
	for _, m := range ms {
		if !strings.HasPrefix(m.Name, "gone_") || m.Value != 0 || m.Help == "" {
			t.Errorf("unexpected metric %+v", m)
		}
	}
	_ = db.Close()
	if _, err := storageSource(idx, realClock{})(context.Background()); err == nil {
		t.Fatal("expected an error on a closed database")
	}
}

// getBody fetches url with header and returns the body of a 200 response.
//
// Parameters:
//   - t: the test handle.
//   - url: the URL to fetch.
//   - header: request headers.
//
// Returns:
//   - string: the response body.
func getBody(t *testing.T, url string, header http.Header) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, %v", url, resp.StatusCode, err)
	}
	return string(b)
}

// TestStartMetrics_ListenerFailureLogged verifies listener failures are logged.
//
// Parameters:
//   - t: the test handle.
func TestStartMetrics_ListenerFailureLogged(t *testing.T) {
	logs := captureLogs(t)
	mgr := startMetricsForTest(t, holdAddr(t), "tok")
	defer mgr.Stop(context.Background())
	waitForMetricsLog(t, logs)
}

// TestStartMetrics_SchemaFailure verifies schema errors are returned.
//
// Parameters:
//   - t: the test handle.
func TestStartMetrics_SchemaFailure(t *testing.T) {
	db, _, _ := testStorage(t)
	_ = db.Close()
	if _, err := startMetrics(context.Background(), db, &config.Config{}); err == nil {
		t.Fatal("expected schema init error on closed database")
	}
}

// startMetricsForTest starts metrics with a fresh test database.
//
// Parameters:
//   - t: the test handle.
//   - addr: the metrics listen address.
//   - token: the metrics bearer token.
//
// Returns:
//   - *metrics.Manager: the started manager.
func startMetricsForTest(t *testing.T, addr, token string) *metrics.Manager {
	t.Helper()
	db, _, _ := testStorage(t)
	mgr, err := startMetrics(context.Background(), db, &config.Config{MetricsAddr: addr, MetricsToken: token})
	if err != nil {
		t.Fatalf("startMetrics: %v", err)
	}
	return mgr
}

// waitForMetricsLog waits for the async metrics listener error log.
//
// Parameters:
//   - t: the test handle.
//   - logs: the captured log buffer.
func waitForMetricsLog(t *testing.T, logs *syncBuffer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "metrics server error") {
		if time.Now().After(deadline) {
			t.Fatal("expected metrics server error to be logged")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
