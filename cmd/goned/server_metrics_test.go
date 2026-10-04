package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/config"
	"github.com/haukened/gone/internal/metrics"
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
	db, _, _ := testStorage(t)
	mgr, err := startMetrics(ctx, db, &config.Config{MetricsAddr: addr, MetricsToken: "tok"})
	if err != nil {
		t.Fatalf("startMetrics: %v", err)
	}
	defer mgr.Stop(context.Background())
	waitFor(t, "http://"+addr+"/", http.Header{"Authorization": {"Bearer " + "tok"}}, http.StatusOK)
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
