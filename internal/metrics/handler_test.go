package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeSnapshot struct {
	c   map[string]int64
	s   map[string]summaryAgg
	err error
}

func (f *fakeSnapshot) Snapshot(ctx context.Context) (map[string]int64, map[string]summaryAgg, error) {
	return f.c, f.s, f.err
}

func TestHandlerAuth(t *testing.T) {
	f := &fakeSnapshot{c: map[string]int64{"a": 1}, s: map[string]summaryAgg{"x": {count: 2, sum: 5, min: 2, max: 3}}}
	h := Handler(f, "tok")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rw := httptest.NewRecorder()
	h(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", rw.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req2.Header.Set("Authorization", "Bearer tok")
	rw2 := httptest.NewRecorder()
	h(rw2, req2)
	if rw2.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw2.Code)
	}
	var decoded struct {
		Counters  map[string]int64            `json:"counters"`
		Summaries map[string]map[string]int64 `json:"summaries"`
	}
	if err := json.Unmarshal(rw2.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Counters["a"] != 1 {
		t.Fatalf("counter mismatch")
	}
	if v := decoded.Summaries["x"]; v["count"] != 2 || v["sum"] != 5 || v["min"] != 2 || v["max"] != 3 {
		t.Fatalf("summary mismatch: %+v", v)
	}
}

func TestHandlerNoToken(t *testing.T) {
	f := &fakeSnapshot{c: map[string]int64{"c": 10}, s: map[string]summaryAgg{}}
	h := Handler(f, "")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
}
