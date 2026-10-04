package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTLS starts a TLS test server and returns a client trusting it.
//
// Parameters:
//   - t: test handle.
//   - h: server handler.
//
// Returns the client and server.
func newTLS(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	c, err := New(srv.URL, Options{RootCAs: pool, UserAgent: "gone-test"})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		in        string
		allowHTTP bool
		want      string
		err       error
	}{
		{"https://Gone.Example", false, "https://gone.example", nil},
		{"https://gone.example/", false, "https://gone.example", nil},
		{"HTTPS://gone.example:8443", false, "https://gone.example:8443", nil},
		{"http://localhost:8080", true, "http://localhost:8080", nil},
		{"http://localhost:8080", false, "", ErrInsecureOrigin},
		{"ftp://gone.example", false, "", ErrInvalidOrigin},
		{"https://gone.example/path", false, "", ErrInvalidOrigin},
		{"https://gone.example?q=1", false, "", ErrInvalidOrigin},
		{"https://gone.example?", false, "", ErrInvalidOrigin},
		{"https://gone.example#frag", false, "", ErrInvalidOrigin},
		{"https://user@gone.example", false, "", ErrInvalidOrigin},
		{"gone.example", false, "", ErrInvalidOrigin},
		{"https:opaque", false, "", ErrInvalidOrigin},
		{"https://%zz", false, "", ErrInvalidOrigin},
		{"", false, "", ErrInvalidOrigin},
	}
	for _, tc := range tests {
		got, err := NormalizeOrigin(tc.in, tc.allowHTTP)
		if !errors.Is(err, tc.err) || got != tc.want {
			t.Errorf("NormalizeOrigin(%q, %v) = %q, %v; want %q, %v", tc.in, tc.allowHTTP, got, err, tc.want, tc.err)
		}
	}
}

func TestNewRejectsBadOrigin(t *testing.T) {
	if _, err := New("http://x", Options{}); !errors.Is(err, ErrInsecureOrigin) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewDefaults(t *testing.T) {
	c, err := New("https://gone.example", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if c.hc.Timeout != DefaultTimeout {
		t.Fatalf("timeout = %v", c.hc.Timeout)
	}
	if c.Origin() != "https://gone.example" {
		t.Fatalf("origin = %q", c.Origin())
	}
	tr := c.hc.Transport.(*http.Transport)
	if tr.TLSClientConfig.MinVersion < 0x0303 {
		t.Fatal("TLS minimum below 1.2")
	}
	c2, _ := New("https://gone.example", Options{Timeout: time.Second})
	if c2.hc.Timeout != time.Second {
		t.Fatalf("timeout = %v", c2.hc.Timeout)
	}
}

func TestUserAgentAndNoRedirect(t *testing.T) {
	var ua string
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		http.Redirect(w, r, "https://evil.example/", http.StatusFound)
	})
	err := c.Revoke(context.Background(), testID, testManage)
	if !errors.Is(err, ErrServer) {
		t.Fatalf("redirect err = %v, want ErrServer", err)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusFound {
		t.Fatalf("status error = %v", err)
	}
	if ua != "gone-test" {
		t.Fatalf("user agent = %q", ua)
	}
}

func TestUntrustedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Revoke(context.Background(), testID, testManage); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v, want ErrNetwork", err)
	}
}

func TestPlainHTTPAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, Options{AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Revoke(context.Background(), testID, testManage); err != nil {
		t.Fatal(err)
	}
}

func TestDoBadRequest(t *testing.T) {
	c, _ := New("https://gone.example", Options{})
	resp, err := c.do(context.Background(), "BAD METHOD", "/", nil, nil)
	if resp != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
}

// fakeResp builds a response for readBounded tests.
//
// Parameters:
//   - t: test handle.
//   - body: body text.
//   - cl: Content-Length value; -1 for unknown.
//
// Returns the response.
func fakeResp(t *testing.T, body string, cl int64) *http.Response {
	t.Helper()
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body)), ContentLength: cl}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return resp
}

// errReader fails every read.
type errReader struct{}

// Read always fails.
//
// Parameters:
//   - p: unused.
//
// Returns 0 and an error.
func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// fakeErrResp builds a response whose body read fails.
//
// Parameters:
//   - t: test handle.
//
// Returns the response.
func fakeErrResp(t *testing.T) *http.Response {
	t.Helper()
	resp := &http.Response{Body: io.NopCloser(errReader{}), ContentLength: -1}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return resp
}

func TestReadBounded(t *testing.T) {
	tests := []struct {
		name string
		body string
		cl   int64
		lim  int64
		want string
		err  error
	}{
		{"ok known", "abc", 3, 10, "abc", nil},
		{"ok unknown", "abc", -1, 10, "abc", nil},
		{"declared too large", "abc", 11, 10, "", ErrTooLarge},
		{"streamed too large", "abcdefghijk", -1, 10, "", ErrTooLarge},
		{"short", "ab", 3, 10, "", ErrIntegrity},
		{"read error", "", -1, 10, "", ErrNetwork},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := fakeResp(t, tc.body, tc.cl) //nolint:bodyclose // readBounded closes the response body.
			if tc.name == "read error" {
				resp = fakeErrResp(t) //nolint:bodyclose // readBounded closes the response body.
			}
			got, err := readBounded(resp, tc.lim)
			if !errors.Is(err, tc.err) || string(got) != tc.want {
				t.Fatalf("got %q, %v; want %q, %v", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestSingle(t *testing.T) {
	h := http.Header{}
	if _, ok := single(h, "X-A"); ok {
		t.Fatal("absent header reported present")
	}
	h.Add("X-A", "1")
	if v, ok := single(h, "X-A"); !ok || v != "1" {
		t.Fatal("single header not found")
	}
	h.Add("X-A", "2")
	if _, ok := single(h, "X-A"); ok {
		t.Fatal("repeated header accepted")
	}
}
