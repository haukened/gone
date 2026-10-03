package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/httpx"
	"github.com/haukened/gone/internal/ratelimit"
)

// createManaged creates a secret and returns its ID and manage token.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//
// Returns:
//   - string: the new secret's ID.
//   - string: the sender's manage token.
func createManaged(t *testing.T, base string) (string, string) {
	t.Helper()
	resp := create(t, base)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID          string `json:"id"`
		ManageToken string `json:"manage_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" || len(created.ManageToken) != 43 {
		t.Fatalf("create response id %q, manage_token length %d", created.ID, len(created.ManageToken))
	}
	return created.ID, created.ManageToken
}

// manageReq sends a status or revoke request with the given manage token.
//
// Parameters:
//   - t: the test.
//   - method: http.MethodGet for status, http.MethodPost for revoke.
//   - base: the server URL.
//   - id: the secret ID.
//   - token: the manage token, or "" to send no header.
//
// Returns:
//   - *http.Response: the server's response.
func manageReq(t *testing.T, method, base, id, token string) *http.Response {
	t.Helper()
	action := "status"
	if method == http.MethodPost {
		action = "revoke"
	}
	req, err := http.NewRequest(method, base+"/api/secret/"+id+"/"+action, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set(httpx.HeaderManage, token)
	}
	return do(t, req)
}

// status returns the status code of GET /api/secret/{id}/status.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - token: the manage token.
//
// Returns:
//   - int: the response status code.
func status(t *testing.T, base, id, token string) int {
	t.Helper()
	return manageReq(t, http.MethodGet, base, id, token).StatusCode
}

// revoke returns the status code of POST /api/secret/{id}/revoke.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - token: the manage token.
//
// Returns:
//   - int: the response status code.
func revoke(t *testing.T, base, id, token string) int {
	t.Helper()
	return manageReq(t, http.MethodPost, base, id, token).StatusCode
}

// otherToken returns a well-formed manage token that differs from token.
//
// Parameters:
//   - token: a 43-character manage token.
//
// Returns:
//   - string: a different token of the same shape.
func otherToken(token string) string {
	if token[0] == 'A' {
		return "B" + token[1:]
	}
	return "A" + token[1:]
}

func TestManageStatusThenRevoke(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)

	resp := manageReq(t, http.MethodGet, srv.URL, id, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	var body struct {
		State     string    `json:"state"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.State != "pending" || !body.ExpiresAt.After(body.CreatedAt) {
		t.Fatalf("status body = %+v, want pending with expires after created", body)
	}

	if got := revoke(t, srv.URL, id, token); got != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", got)
	}
	if got := read(t, srv.URL, id, "").StatusCode; got != http.StatusNotFound {
		t.Fatalf("claim after revoke = %d, want 404", got)
	}
	if got := status(t, srv.URL, id, token); got != http.StatusNotFound {
		t.Fatalf("status after revoke = %d, want 404", got)
	}
	if got := revoke(t, srv.URL, id, token); got != http.StatusNotFound {
		t.Fatalf("second revoke = %d, want 404", got)
	}
}

func TestManageStatusAcrossClaimAndAck(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)

	claim := claimToken(t, srv.URL, id)
	if got := status(t, srv.URL, id, token); got != http.StatusOK {
		t.Fatalf("status during lease = %d, want 200", got)
	}
	if got := ack(t, srv.URL, id, claim); got != http.StatusNoContent {
		t.Fatalf("ack = %d, want 204", got)
	}
	if got := status(t, srv.URL, id, token); got != http.StatusNotFound {
		t.Fatalf("status after ack = %d, want 404", got)
	}
}

func TestManageRevokeWinsDuringLease(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)

	claim := claimToken(t, srv.URL, id)
	if got := revoke(t, srv.URL, id, token); got != http.StatusNoContent {
		t.Fatalf("revoke during lease = %d, want 204", got)
	}
	if got := ack(t, srv.URL, id, claim); got != http.StatusNotFound {
		t.Fatalf("ack after revoke = %d, want 404", got)
	}
	if got := read(t, srv.URL, id, claim).StatusCode; got != http.StatusNotFound {
		t.Fatalf("re-read after revoke = %d, want 404", got)
	}
}

func TestManageRejectsWrongOrMissingToken(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)
	wrong := otherToken(token)

	cases := []struct {
		name   string
		method string
		token  string
		want   int
	}{
		{"status missing", http.MethodGet, "", http.StatusBadRequest},
		{"status malformed", http.MethodGet, "short", http.StatusBadRequest},
		{"status wrong", http.MethodGet, wrong, http.StatusNotFound},
		{"revoke missing", http.MethodPost, "", http.StatusBadRequest},
		{"revoke wrong", http.MethodPost, wrong, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := manageReq(t, tc.method, srv.URL, id, tc.token).StatusCode; got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}

	// None of the rejected attempts may have touched the secret.
	if got := status(t, srv.URL, id, token); got != http.StatusOK {
		t.Fatalf("status with real token = %d, want 200", got)
	}
	claim := claimToken(t, srv.URL, id)
	if got := ack(t, srv.URL, id, claim); got != http.StatusNoContent {
		t.Fatalf("ack = %d, want 204", got)
	}
}

func TestManageSharesReadBudget(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{read: ratelimit.Rate{Count: 2, Per: time.Hour}, burst: 2})
	id, token := createManaged(t, srv.URL)

	if got := status(t, srv.URL, id, token); got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	if got := read(t, srv.URL, id, "").StatusCode; got != http.StatusOK {
		t.Fatalf("claim = %d, want 200", got)
	}
	assertLimited(t, manageReq(t, http.MethodPost, srv.URL, id, token))
}
