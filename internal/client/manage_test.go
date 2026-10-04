package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestStatus checks status requests, response decoding and error mapping.
//
// Parameters:
//   - t: the test handle.
func TestStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"pending", http.StatusOK, `{"state":"pending","created_at":"2030-01-01T00:00:00Z","expires_at":"2030-01-02T00:00:00Z"}`, nil},
		{"other state", http.StatusOK, `{"state":"opened"}`, ErrProtocol},
		{"bad json", http.StatusOK, `nope`, ErrProtocol},
		{"huge", http.StatusOK, string(make([]byte, maxJSONBody+1)), ErrTooLarge},
		{"not found", http.StatusNotFound, ``, ErrNotFound},
		{"bad request", http.StatusBadRequest, ``, ErrRejected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkStatusCase(t, tc.status, tc.body, tc.want)
		})
	}
}

// checkStatusCase verifies one Status client scenario.
//
// Parameters:
//   - t: the test handle.
//   - status: HTTP status returned by the test server.
//   - body: HTTP response body returned by the test server.
//   - want: expected client error.
func checkStatusCase(t *testing.T, status int, body string, want error) {
	t.Helper()
	var path, tok, method string
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		path, tok, method = r.URL.Path, r.Header.Get(headerManage), r.Method
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	st, err := c.Status(context.Background(), testID, testManage)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	requireStatusRequest(t, method, path, tok)
	if want == nil {
		requirePendingStatus(t, st)
	}
}

// requireStatusRequest verifies the request details sent by Status.
//
// Parameters:
//   - t: the test handle.
//   - method: captured request method.
//   - path: captured request path.
//   - token: captured management token header.
func requireStatusRequest(t *testing.T, method string, path string, token string) {
	t.Helper()
	if method != http.MethodGet {
		t.Fatalf("method = %s", method)
	}
	if path != "/api/secret/"+testID.String()+"/status" {
		t.Fatalf("path = %s", path)
	}
	if token != testManage.String() {
		t.Fatalf("token = %s", token)
	}
}

// requirePendingStatus verifies the decoded pending status payload.
//
// Parameters:
//   - t: the test handle.
//   - st: status payload returned by the client.
func requirePendingStatus(t *testing.T, st Status) {
	t.Helper()
	if st.State != "pending" {
		t.Fatalf("state = %s", st.State)
	}
	if !st.ExpiresAt.Equal(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expires_at = %s", st.ExpiresAt)
	}
}

// TestRevoke checks revoke requests and network error mapping.
//
// Parameters:
//   - t: the test handle.
func TestRevoke(t *testing.T) {
	var path, method string
	code := http.StatusNoContent
	c, srv := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		w.WriteHeader(code)
	})
	if err := c.Revoke(context.Background(), testID, testManage); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/api/secret/"+testID.String()+"/revoke" {
		t.Fatalf("request = %s %s", method, path)
	}
	code = http.StatusNotFound
	if err := c.Revoke(context.Background(), testID, testManage); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	srv.Close()
	if err := c.Revoke(context.Background(), testID, testManage); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Status(context.Background(), testID, testManage); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
}
