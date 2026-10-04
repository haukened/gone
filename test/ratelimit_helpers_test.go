package integration_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/haukened/gone/internal/httpx"
)

// read issues GET /api/secret/{id} with an optional claim token.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret id.
//   - claim: a claim token, or "" for none.
//
// Returns:
//   - *http.Response: the server's response.
func read(t *testing.T, base, id, claim string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/api/secret/"+id, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if claim != "" {
		req.Header.Set(httpx.HeaderClaim, claim)
	}
	return do(t, req)
}

// createStatus posts a create request and closes the response body.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//
// Returns:
//   - int: the response status code.
func createStatus(t *testing.T, base string) int {
	t.Helper()
	resp := create(t, base)
	defer closeRatelimitResponse(t, resp)
	return resp.StatusCode
}

// readRateStatus sends a read request and closes the response body.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - claim: the claim token, or "".
//
// Returns:
//   - int: the response status code.
func readRateStatus(t *testing.T, base, id, claim string) int {
	t.Helper()
	resp := read(t, base, id, claim)
	defer closeRatelimitResponse(t, resp)
	return resp.StatusCode
}

// forwardedCreateStatus sends a proxied create request and closes the body.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - forwardedFor: the X-Forwarded-For value.
//
// Returns:
//   - int: the response status code.
func forwardedCreateStatus(t *testing.T, base, forwardedFor string) int {
	t.Helper()
	resp := do(t, createReq(t, base, forwardedFor))
	defer closeRatelimitResponse(t, resp)
	return resp.StatusCode
}

// assertCreateLimited checks a rate-limited create response.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
func assertCreateLimited(t *testing.T, base string) {
	t.Helper()
	resp := create(t, base)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	assertLimited(t, resp)
}

// assertReadLimited checks a rate-limited read response.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - id: the secret ID.
//   - claim: the claim token, or "".
func assertReadLimited(t *testing.T, base, id, claim string) {
	t.Helper()
	resp := read(t, base, id, claim)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	assertLimited(t, resp)
}

// assertForwardedCreateLimited checks a rate-limited proxied create response.
//
// Parameters:
//   - t: the test.
//   - base: the server URL.
//   - forwardedFor: the X-Forwarded-For value.
func assertForwardedCreateLimited(t *testing.T, base, forwardedFor string) {
	t.Helper()
	resp := do(t, createReq(t, base, forwardedFor))
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	assertLimited(t, resp)
}

// closeRatelimitResponse closes resp and fails t on close errors.
//
// Parameters:
//   - t: the test.
//   - resp: the response to close.
func closeRatelimitResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
}

// assertLimited checks that resp is a 429 with a positive whole-second
// Retry-After header.
//
// Parameters:
//   - t: the test.
//   - resp: the response to check.
func assertLimited(t *testing.T, resp *http.Response) {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || secs < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer", resp.Header.Get("Retry-After"))
	}
}
