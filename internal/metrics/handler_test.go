package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeSnapshot struct {
	c   map[string]int64
	s   map[string]SummaryAgg
	err error
}

// Snapshot returns the fake snapshot values configured for a test.
func (f *fakeSnapshot) Snapshot(_ context.Context) (map[string]int64, map[string]SummaryAgg, error) {
	return f.c, f.s, f.err
}

func TestHandlerAuth(t *testing.T) {
	f := &fakeSnapshot{c: map[string]int64{"a": 1}, s: map[string]SummaryAgg{"x": {Count: 2, Sum: 5, Min: 2, Max: 3}}}
	h := Handler(f, "tok")
	for _, tc := range handlerUnauthorizedCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertMetricsHandlerStatus(t, h, tc.header, http.StatusUnauthorized)
		})
	}
	decoded := decodeMetricsHandlerOK(t, h, "Bearer tok")
	if decoded.Counters["a"] != 1 {
		t.Fatalf("counter mismatch")
	}
	if v := decoded.Summaries["x"]; v["count"] != 2 || v["sum"] != 5 || v["min"] != 2 || v["max"] != 3 {
		t.Fatalf("summary mismatch: %+v", v)
	}
}

type handlerUnauthorizedCase struct {
	name   string
	header string
}

// handlerUnauthorizedCases returns Authorization headers rejected by Handler.
func handlerUnauthorizedCases() []handlerUnauthorizedCase {
	return []handlerUnauthorizedCase{
		{name: "missing"},
		{name: "malformed", header: "Basic tok"},
		{name: "empty bearer", header: "Bearer "},
		{name: "wrong same length", header: "Bearer bad"},
		{name: "wrong different length", header: "Bearer longer"},
	}
}

// assertMetricsHandlerStatus verifies a metrics handler response status.
func assertMetricsHandlerStatus(t *testing.T, h http.HandlerFunc, header string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rw := httptest.NewRecorder()
	h(rw, req)
	if rw.Code != want {
		t.Fatalf("status = %d, want %d", rw.Code, want)
	}
	return rw
}

type decodedMetricsResponse struct {
	Counters  map[string]int64            `json:"counters"`
	Summaries map[string]map[string]int64 `json:"summaries"`
}

// decodeMetricsHandlerOK verifies success and decodes the metrics body.
func decodeMetricsHandlerOK(t *testing.T, h http.HandlerFunc, header string) decodedMetricsResponse {
	t.Helper()
	rw := assertMetricsHandlerStatus(t, h, header, http.StatusOK)
	var decoded decodedMetricsResponse
	if err := json.Unmarshal(rw.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

func TestHandlerEmptyConfiguredTokenFailsClosed(t *testing.T) {
	f := &fakeSnapshot{c: map[string]int64{"c": 10}, s: map[string]SummaryAgg{}}
	h := Handler(f, "")
	assertMetricsHandlerStatus(t, h, "Bearer tok", http.StatusUnauthorized)
}

func TestHandlerSnapshotError(t *testing.T) {
	h := Handler(&fakeSnapshot{err: errors.New("snapshot failed")}, "tok")
	assertMetricsHandlerStatus(t, h, "Bearer tok", http.StatusInternalServerError)
}
