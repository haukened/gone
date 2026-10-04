package client

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStatusErrorMapping(t *testing.T) {
	tests := []struct {
		code int
		want error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusRequestEntityTooLarge, ErrTooLarge},
		{http.StatusBadRequest, ErrRejected},
		{http.StatusForbidden, ErrRejected},
		{http.StatusInternalServerError, ErrServer},
		{http.StatusBadGateway, ErrServer},
		{http.StatusMovedPermanently, ErrServer},
		{http.StatusOK, ErrServer},
		{http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tc := range tests {
		err := statusError(&http.Response{StatusCode: tc.code, Header: http.Header{}})
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: err = %v, want %v", tc.code, err, tc.want)
		}
	}
	err := statusError(&http.Response{StatusCode: 404, Header: http.Header{}})
	if err.Error() != "not found (HTTP 404)" {
		t.Fatalf("message = %q", err)
	}
}

func TestRateLimitError(t *testing.T) {
	tests := []struct {
		header string
		want   time.Duration
		msg    string
	}{
		{"30", 30 * time.Second, "rate limited; retry after 30s"},
		{"", 0, "rate limited"},
		{"-1", 0, "rate limited"},
		{"Wed, 21 Oct 2015 07:28:00 GMT", 0, "rate limited"},
		{"99999999", maxRetryAfter, "rate limited; retry after 24h0m0s"},
		{"99999999999999999999", 0, "rate limited"},
	}
	for _, tc := range tests {
		h := http.Header{}
		if tc.header != "" {
			h.Set("Retry-After", tc.header)
		}
		err := statusError(&http.Response{StatusCode: http.StatusTooManyRequests, Header: h})
		var rl *RateLimitError
		if !errors.As(err, &rl) || rl.RetryAfter != tc.want || err.Error() != tc.msg {
			t.Errorf("Retry-After %q: %v (%v)", tc.header, err, rl)
		}
	}
}

func TestWrapNetwork(t *testing.T) {
	inner := errors.New("dial failed")
	err := wrapNetwork(inner)
	if !errors.Is(err, ErrNetwork) || !errors.Is(err, inner) || !strings.Contains(err.Error(), "dial failed") {
		t.Fatalf("err = %v", err)
	}
}
