package httpx_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/httpx"
)

// readAll mimics the store: it insists on exactly size bytes from the body.
func readAll(ct io.Reader, size int64) error {
	_, err := io.ReadFull(ct, make([]byte, size))
	return err
}

// captureJSONLogs redirects the default slog logger into a buffer for the test.
func captureJSONLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

// assertNoErrorLog fails if the captured JSON logs hold an ERROR record.
func assertNoErrorLog(t *testing.T, logs *bytes.Buffer) {
	t.Helper()
	if strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("unexpected ERROR log line: %s", logs.String())
	}
}

// createReq builds a valid create request whose Content-Length is 10.
func createReq(ctx context.Context, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/secret", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Length", "10")
	req.Header.Set("X-Gone-Version", "1")
	req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
	req.Header.Set("X-Gone-TTL", "5m")
	return req
}

// TestCreate_ShortBodyIs400 checks that a body shorter than Content-Length
// (partial or empty) is a 400 "short body", not a 500 with an ERROR log.
func TestCreate_ShortBodyIs400(t *testing.T) {
	for name, body := range map[string]string{"partial": "abc", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			logs := captureJSONLogs(t)
			svc := mockService{createFn: func(_ context.Context, ct io.Reader, size int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
				return app.Created{}, readAll(ct, size)
			}}
			rr := httptest.NewRecorder()
			httpx.New(svc, 1024, nil).Router().ServeHTTP(rr, createReq(context.Background(), body))
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"short body"`) {
				t.Fatalf("got %d %s, want 400 short body", rr.Code, rr.Body.String())
			}
			assertNoErrorLog(t, logs)
		})
	}
}

// TestCreate_ClientGoneWritesNothing checks that a canceled or timed-out
// request context yields a bare 499 and no ERROR log, whatever error the
// service reports.
func TestCreate_ClientGoneWritesNothing(t *testing.T) {
	cases := map[string]func() (context.Context, context.CancelFunc){
		"canceled": func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		},
		"deadline": func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		},
	}
	errs := map[string]error{"ctx": context.Canceled, "read": errors.New("read tcp: connection reset"), "short": io.ErrUnexpectedEOF}
	for cname, mk := range cases {
		for ename, serr := range errs {
			t.Run(cname+"/"+ename, func(t *testing.T) {
				logs := captureJSONLogs(t)
				ctx, cancel := mk()
				defer cancel()
				svc := mockService{createFn: func(context.Context, io.Reader, int64, uint8, string, time.Duration) (app.Created, error) {
					return app.Created{}, serr
				}}
				rr := httptest.NewRecorder()
				httpx.New(svc, 1024, nil).Router().ServeHTTP(rr, createReq(ctx, "abcdefghij"))
				if rr.Code != 499 || rr.Body.Len() != 0 {
					t.Fatalf("got %d %q, want a bare 499", rr.Code, rr.Body.String())
				}
				assertNoErrorLog(t, logs)
			})
		}
	}
}

// TestFill_ShortBodyAndClientGone covers the same two cases on the reply path.
func TestFill_ShortBodyAndClientGone(t *testing.T) {
	put := func(ctx context.Context, svc *fakeRequests) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/request/"+reqID+"/reply", strings.NewReader("abc")).WithContext(ctx)
		req.Header.Set("Content-Length", "10")
		req.Header.Set("X-Gone-Version", "3")
		req.Header.Set("X-Gone-Nonce", domain.EncodeB64URL(make([]byte, domain.NonceSize)))
		req.Header.Set(httpx.HeaderFill, "tok")
		rr := httptest.NewRecorder()
		h := httpx.New(mockService{}, 1024, nil)
		h.Requests = svc
		h.Router().ServeHTTP(rr, req)
		return rr
	}
	logs := captureJSONLogs(t)
	rr := put(context.Background(), &fakeRequests{err: io.ErrUnexpectedEOF})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"short body"`) {
		t.Fatalf("got %d %s, want 400 short body", rr.Code, rr.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rr = put(ctx, &fakeRequests{err: context.Canceled})
	if rr.Code != 499 || rr.Body.Len() != 0 {
		t.Fatalf("got %d %q, want a bare 499", rr.Code, rr.Body.String())
	}
	assertNoErrorLog(t, logs)
}
