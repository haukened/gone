package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/httpx"
)

// createID creates a secret and returns its ID.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//
// Returns:
//   - string: the new secret's ID.
func createID(t *testing.T, base string) string {
	t.Helper()
	resp := create(t, base)
	defer closeRatelimitResponse(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil || created.ID == "" {
		t.Fatalf("decode create response: %v (id %q)", err, created.ID)
	}
	return created.ID
}

// claimToken claims a secret and returns the claim token.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//
// Returns:
//   - string: the claim token from the response.
func claimToken(t *testing.T, base, id string) string {
	t.Helper()
	resp := read(t, base, id, "")
	defer closeRatelimitResponse(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("claim status = %d, want 200", resp.StatusCode)
	}
	token := resp.Header.Get(httpx.HeaderClaim)
	if token == "" {
		t.Fatal("claim response missing claim token")
	}
	return token
}

// ack acknowledges a claimed secret.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - token: the claim token.
//
// Returns:
//   - int: the response status code.
func ack(t *testing.T, base, id, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, base+"/api/secret/"+id, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(httpx.HeaderClaim, token)
	resp := do(t, req)
	defer closeRatelimitResponse(t, resp)
	return resp.StatusCode
}

// TestFullFlowUnderDefaults verifies create, claim, ack, and gone states.
//
// Parameters:
//   - t: the test handle.
func TestFullFlowUnderDefaults(t *testing.T) {
	cfg := config.DefaultAppConfig
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{create: cfg.CreateRate, read: cfg.ReadRate, burst: cfg.RateBurst})

	id := createID(t, srv.URL)
	token := claimToken(t, srv.URL, id)
	if got := ack(t, srv.URL, id, token); got != http.StatusNoContent {
		t.Fatalf("ack status = %d, want 204", got)
	}
	if got := readRateStatus(t, srv.URL, id, ""); got != http.StatusNotFound {
		t.Fatalf("read after ack status = %d, want 404", got)
	}
}
