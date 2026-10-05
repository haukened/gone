package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/httpx"
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
	defer closeManageResponse(t, resp)
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
	return manageReqStatus(t, http.MethodGet, base, id, token)
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
	return manageReqStatus(t, http.MethodPost, base, id, token)
}

// manageReqStatus sends a manage request and closes the response body.
//
// Parameters:
//   - t: the test.
//   - method: http.MethodGet for status, http.MethodPost for revoke.
//   - base: the server URL.
//   - id: the secret ID.
//   - token: the manage token, or "" to send no header.
//
// Returns:
//   - int: the response status code.
func manageReqStatus(t *testing.T, method, base, id, token string) int {
	t.Helper()
	resp := manageReq(t, method, base, id, token)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	return resp.StatusCode
}

// closeManageResponse closes resp and fails t on close errors.
//
// Parameters:
//   - t: the test.
//   - resp: the response to close.
func closeManageResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
}

// readSecretStatus returns GET /api/secret/{id}'s status code.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - claim: the claim token, or "".
//
// Returns:
//   - int: the response status code.
func readSecretStatus(t *testing.T, base, id, claim string) int {
	t.Helper()
	resp := read(t, base, id, claim)
	defer closeManageResponse(t, resp)
	return resp.StatusCode
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

// manageStatusBody is the successful status response shape.
type manageStatusBody struct {
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// requireManageStatusOK validates a pending manage status response.
//
// Parameters:
//   - t: the test.
//   - resp: the status response.
func requireManageStatusOK(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	body := decodeManageStatus(t, resp)
	if body.State != "pending" || !body.ExpiresAt.After(body.CreatedAt) {
		t.Fatalf("status body = %+v, want pending with expires after created", body)
	}
}

// decodeManageStatus decodes a successful status response body.
//
// Parameters:
//   - t: the test.
//   - resp: the status response.
//
// Returns:
//   - manageStatusBody: the decoded response body.
func decodeManageStatus(t *testing.T, resp *http.Response) manageStatusBody {
	t.Helper()
	var body manageStatusBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return body
}

// requireManageStatusCode fails t when got does not equal want.
//
// Parameters:
//   - t: the test.
//   - label: the operation being checked.
//   - got: the observed status code.
//   - want: the expected status code.
func requireManageStatusCode(t *testing.T, label string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %d, want %d", label, got, want)
	}
}
