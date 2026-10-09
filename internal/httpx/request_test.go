package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/httpx"
)

const reqID = "0123456789abcdef0123456789abcdef"

// fakeRequests implements httpx.RequestPort, recording the tokens it sees.
type fakeRequests struct {
	err     error
	status  app.RequestStatus
	claim   app.ClaimResult
	expires time.Time
	got     struct {
		fill, manage, claim, nonce, body string
		version                          uint8
		size                             int64
		ttl                              time.Duration
	}
}

func (f *fakeRequests) CreateRequest(_ context.Context, ttl time.Duration) (app.CreatedRequest, error) {
	f.got.ttl = ttl
	if f.err != nil {
		return app.CreatedRequest{}, f.err
	}
	return app.CreatedRequest{ID: reqID, ExpiresAt: f.expires, ManageToken: "m", FillToken: "f"}, nil
}

func (f *fakeRequests) RequestOpen(_ context.Context, _, fill string) (time.Time, error) {
	f.got.fill = fill
	return f.expires, f.err
}

func (f *fakeRequests) Fill(_ context.Context, _, fill string, ct io.Reader, size int64, version uint8, nonce string) (time.Time, error) {
	b, _ := io.ReadAll(ct)
	f.got.fill, f.got.body, f.got.size, f.got.version, f.got.nonce = fill, string(b), size, version, nonce
	return f.expires, f.err
}

func (f *fakeRequests) RequestStatus(_ context.Context, _, token string) (app.RequestStatus, error) {
	f.got.manage = token
	return f.status, f.err
}

func (f *fakeRequests) ClaimReply(_ context.Context, _, manage, claim string) (app.ClaimResult, error) {
	f.got.manage, f.got.claim = manage, claim
	return f.claim, f.err
}

func (f *fakeRequests) AckReply(_ context.Context, _, claim string) error {
	f.got.claim = claim
	return f.err
}

func (f *fakeRequests) CancelRequest(_ context.Context, _, token string) error {
	f.got.manage = token
	return f.err
}

func reqServe(t *testing.T, f *fakeRequests, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	h := httpx.New(mockService{}, 64, nil)
	if f != nil {
		h.Requests = f
	}
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	return w
}

func reqWith(method, path string, body io.Reader, headers ...string) *http.Request {
	r := httptest.NewRequest(method, path, body)
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

func TestCreateRequestHandler(t *testing.T) {
	f := &fakeRequests{expires: time.Unix(1700000000, 0)}
	w := reqServe(t, f, reqWith(http.MethodPost, "/api/request", nil, "X-Gone-TTL", "30m"))
	if w.Code != http.StatusCreated || f.got.ttl != 30*time.Minute {
		t.Fatalf("code=%d ttl=%v body=%s", w.Code, f.got.ttl, w.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["id"] != reqID || body["manage_token"] != "m" || body["fill_token"] != "f" {
		t.Fatalf("body = %v, %v", body, err)
	}
	cases := []struct {
		name string
		r    *http.Request
		code int
	}{
		{"missing ttl", reqWith(http.MethodPost, "/api/request", nil), http.StatusBadRequest},
		{"repeated ttl", reqWith(http.MethodPost, "/api/request", nil, "X-Gone-TTL", "1m", "X-Gone-TTL", "1m"), http.StatusBadRequest},
		{"bad ttl", reqWith(http.MethodPost, "/api/request", nil, "X-Gone-TTL", "soon"), http.StatusBadRequest},
		{"wrong method", reqWith(http.MethodGet, "/api/request", nil), http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		if w := reqServe(t, f, c.r); w.Code != c.code {
			t.Errorf("%s: code=%d want %d", c.name, w.Code, c.code)
		}
	}
	f.err = domain.ErrTTLInvalid
	if w := reqServe(t, f, reqWith(http.MethodPost, "/api/request", nil, "X-Gone-TTL", "9h")); w.Code != http.StatusBadRequest {
		t.Fatalf("service ttl err code=%d", w.Code)
	}
}

func TestRequestRoutesDisabled(t *testing.T) {
	for _, r := range []*http.Request{
		reqWith(http.MethodPost, "/api/request", nil, "X-Gone-TTL", "1m"),
		reqWith(http.MethodGet, "/api/request/"+reqID, nil),
	} {
		if w := reqServe(t, nil, r); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: code=%d", r.Method, r.URL.Path, w.Code)
		}
	}
}

func TestRequestOpenHandler(t *testing.T) {
	f := &fakeRequests{expires: time.Unix(1700000000, 0)}
	w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID, nil, httpx.HeaderFill, "tok"))
	if w.Code != http.StatusOK || f.got.fill != "tok" || !strings.Contains(w.Body.String(), `"state":"open"`) {
		t.Fatalf("code=%d fill=%q body=%s", w.Code, f.got.fill, w.Body)
	}
	reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID, nil, httpx.HeaderFill, "a", httpx.HeaderFill, "b"))
	if f.got.fill != "" {
		t.Fatalf("repeated fill header passed through as %q", f.got.fill)
	}
	f.err = app.ErrNotFound
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID, nil)); w.Code != http.StatusNotFound {
		t.Fatalf("not found code=%d", w.Code)
	}
	f.err = domain.ErrInvalidFill
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID, nil)); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid fill token") {
		t.Fatalf("invalid fill code=%d body=%s", w.Code, w.Body)
	}
}

func TestFillHandler(t *testing.T) {
	nonce := domain.EncodeB64URL(make([]byte, domain.NonceSize))
	fill := func(body string, headers ...string) *http.Request {
		r := reqWith(http.MethodPut, "/api/request/"+reqID+"/reply", strings.NewReader(body), headers...)
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return r
	}
	f := &fakeRequests{expires: time.Unix(1700000000, 0)}
	w := reqServe(t, f, fill("ciphertext", "X-Gone-Version", "3", "X-Gone-Nonce", nonce, httpx.HeaderFill, "tok"))
	if w.Code != http.StatusCreated || f.got.body != "ciphertext" || f.got.version != 3 || f.got.fill != "tok" || f.got.size != 10 {
		t.Fatalf("code=%d got=%+v body=%s", w.Code, f.got, w.Body)
	}
	cases := []struct {
		name string
		r    *http.Request
		code int
	}{
		{"secret version", fill("x", "X-Gone-Version", "1", "X-Gone-Nonce", nonce), http.StatusBadRequest},
		{"bad nonce", fill("x", "X-Gone-Version", "3", "X-Gone-Nonce", "short"), http.StatusBadRequest},
		{"missing headers", fill("x"), http.StatusBadRequest},
		{"too large", fill(strings.Repeat("x", 65), "X-Gone-Version", "3", "X-Gone-Nonce", nonce), http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		if w := reqServe(t, f, c.r); w.Code != c.code {
			t.Errorf("%s: code=%d want %d body=%s", c.name, w.Code, c.code, w.Body)
		}
	}
	noLength := reqWith(http.MethodPut, "/api/request/"+reqID+"/reply", strings.NewReader("x"), "X-Gone-Version", "3", "X-Gone-Nonce", nonce)
	noLength.Header.Del("Content-Length")
	if w := reqServe(t, f, noLength); w.Code != http.StatusLengthRequired {
		t.Errorf("no length: code=%d", w.Code)
	}
	f.err = app.ErrNotFound
	if w := reqServe(t, f, fill("x", "X-Gone-Version", "3", "X-Gone-Nonce", nonce)); w.Code != http.StatusNotFound {
		t.Errorf("closed request: code=%d", w.Code)
	}
}

func TestRequestStatusHandler(t *testing.T) {
	f := &fakeRequests{status: app.RequestStatus{Ready: true, CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0)}}
	w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/status", nil, httpx.HeaderManage, "m"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"ready"`) || f.got.manage != "m" {
		t.Fatalf("code=%d body=%s", w.Code, w.Body)
	}
	f.status.Ready = false
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/status", nil)); !strings.Contains(w.Body.String(), `"state":"waiting"`) {
		t.Fatalf("waiting body=%s", w.Body)
	}
	f.err = app.ErrNotFound
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/status", nil)); w.Code != http.StatusNotFound {
		t.Fatalf("gone code=%d", w.Code)
	}
}

func TestClaimAndAckReplyHandlers(t *testing.T) {
	f := &fakeRequests{claim: app.ClaimResult{
		Claimed: app.Claimed{Meta: app.Meta{Version: 3, NonceB64u: "n"}, Body: io.NopCloser(strings.NewReader("abc")), Size: 3, ClaimedUntil: time.Unix(9, 0)},
		Token:   "claim-token",
	}}
	w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/reply", nil, httpx.HeaderManage, "m"))
	if w.Code != http.StatusOK || w.Body.String() != "abc" || w.Header().Get("X-Gone-Version") != "3" || w.Header().Get(httpx.HeaderClaim) != "claim-token" {
		t.Fatalf("claim code=%d headers=%v body=%q", w.Code, w.Header(), w.Body)
	}
	w = reqServe(t, f, reqWith(http.MethodDelete, "/api/request/"+reqID+"/reply", nil, httpx.HeaderClaim, "claim-token"))
	if w.Code != http.StatusNoContent || f.got.claim != "claim-token" {
		t.Fatalf("ack code=%d claim=%q", w.Code, f.got.claim)
	}
	f.err = app.ErrNotFound
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/reply", nil)); w.Code != http.StatusNotFound {
		t.Fatalf("claim gone code=%d", w.Code)
	}
	if w := reqServe(t, f, reqWith(http.MethodDelete, "/api/request/"+reqID+"/reply", nil)); w.Code != http.StatusNotFound {
		t.Fatalf("ack gone code=%d", w.Code)
	}
	if w := reqServe(t, f, reqWith(http.MethodPost, "/api/request/"+reqID+"/reply", nil)); w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, PUT, DELETE" {
		t.Fatalf("reply method code=%d allow=%q", w.Code, w.Header().Get("Allow"))
	}
}

func TestCancelRequestHandler(t *testing.T) {
	f := &fakeRequests{}
	if w := reqServe(t, f, reqWith(http.MethodPost, "/api/request/"+reqID+"/revoke", nil, httpx.HeaderManage, "m")); w.Code != http.StatusNoContent || f.got.manage != "m" {
		t.Fatalf("cancel code=%d", w.Code)
	}
	f.err = app.ErrNotFound
	if w := reqServe(t, f, reqWith(http.MethodPost, "/api/request/"+reqID+"/revoke", nil)); w.Code != http.StatusNotFound {
		t.Fatalf("cancel gone code=%d", w.Code)
	}
}

func TestRequestRouteErrors(t *testing.T) {
	f := &fakeRequests{}
	cases := []struct {
		r    *http.Request
		code int
	}{
		{reqWith(http.MethodGet, "/api/request/", nil), http.StatusNotFound},
		{reqWith(http.MethodGet, "/api/request/"+reqID+"/nope", nil), http.StatusNotFound},
		{reqWith(http.MethodPost, "/api/request/"+reqID, nil), http.StatusMethodNotAllowed},
		{reqWith(http.MethodPost, "/api/request/"+reqID+"/status", nil), http.StatusMethodNotAllowed},
		{reqWith(http.MethodGet, "/api/request/"+reqID+"/revoke", nil), http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		if w := reqServe(t, f, c.r); w.Code != c.code {
			t.Errorf("%s %s: code=%d want %d", c.r.Method, c.r.URL.Path, w.Code, c.code)
		}
	}
	f.err = errors.New("boom")
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/status", nil)); w.Code != http.StatusInternalServerError {
		t.Errorf("unknown error code=%d", w.Code)
	}
	f.err = app.ErrRequestsDisabled
	if w := reqServe(t, f, reqWith(http.MethodGet, "/api/request/"+reqID+"/status", nil)); w.Code != http.StatusNotFound {
		t.Errorf("disabled code=%d", w.Code)
	}
}

func TestRequestPages(t *testing.T) {
	tests := []struct {
		name, path string
		set        func(h *httpx.Handler)
		code       int
		body       string
	}{
		{"request page", "/request", func(h *httpx.Handler) { h.RequestTmpl = pageTemplate{body: "req"} }, http.StatusOK, "req"},
		{"request page unavailable", "/request", func(*httpx.Handler) {}, http.StatusServiceUnavailable, "request template unavailable"},
		{"detail page", "/request/" + reqID, func(h *httpx.Handler) { h.RequestDetailTmpl = pageTemplate{body: "detail"} }, http.StatusOK, "detail"},
		{"detail bare prefix", "/request/", func(h *httpx.Handler) { h.RequestDetailTmpl = pageTemplate{body: "detail"} }, http.StatusNotFound, "not found"},
		{"reply page", "/reply/" + reqID, func(h *httpx.Handler) { h.ReplyTmpl = pageTemplate{body: "reply"} }, http.StatusOK, "reply"},
		{"reply bare prefix", "/reply/", func(h *httpx.Handler) { h.ReplyTmpl = pageTemplate{body: "reply"} }, http.StatusNotFound, "not found"},
		{"reply unavailable", "/reply/" + reqID, func(*httpx.Handler) {}, http.StatusServiceUnavailable, "reply template unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := httpx.New(mockService{}, 1024, nil)
			tc.set(h)
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("code=%d body=%q", w.Code, w.Body)
			}
		})
	}
}
