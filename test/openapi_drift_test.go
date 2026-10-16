package integration_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/httpx"
	"github.com/haukened/gone/v3/internal/i18n"
	wembed "github.com/haukened/gone/v3/web"
)

const (
	driftID    = "0123456789abcdef0123456789abcdef"
	driftToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 43 base64url characters
	driftNonce = "AAAAAAAAAAAAAAAA"
)

// driftRequest builds a well-formed request for an operation: every header
// any operation reads is present and valid, uploads carry a body.
func driftRequest(op specOp, method string) *http.Request {
	path := strings.NewReplacer("{id}", driftID, "{tag}", "en").Replace(op.path)
	var body io.Reader
	if method == http.MethodPost && op.path == "/api/secret" || method == http.MethodPut {
		body = strings.NewReader("x")
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("X-Gone-Manage", driftToken)
	req.Header.Set("X-Gone-Fill", driftToken)
	req.Header.Set("X-Gone-Claim", driftToken)
	req.Header.Set("X-Gone-Nonce", driftNonce)
	req.Header.Set("X-Gone-TTL", "5m")
	req.Header.Set("X-Gone-Version", "1")
	if op.path == "/api/request/{id}/reply" {
		req.Header.Set("X-Gone-Version", "3")
	}
	if body != nil {
		req.Header.Set("Content-Length", "1")
	}
	return req
}

// driftHandler is the real router over a temporary store, with the i18n
// catalogs mounted and the given readiness probe.
func driftHandler(t *testing.T, ready func(context.Context) error) *httpx.Handler {
	t.Helper()
	cfg := config.DefaultAppConfig
	svc := newServiceAt(t, &cfg, t.TempDir())
	svc.Requests = svc.Store.(app.RequestStore)
	h := httpx.New(svc, cfg.MaxBytes, ready)
	h.MinTTL, h.MaxTTL = cfg.MinTTL, cfg.MaxTTL
	h.Requests = svc
	bundle, err := i18n.Load(wembed.Assets, "messages", i18n.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h.I18n = bundle
	return h
}

// serve runs req through handler.
func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// requireDocumented fails unless op documents the response's status code.
func requireDocumented(t *testing.T, op specOp, rr *httptest.ResponseRecorder, what string) {
	t.Helper()
	if _, ok := op.codes[strconv.Itoa(rr.Code)]; !ok {
		t.Errorf("%s %s: %s returned %d, which the spec does not document (documented: %v)", op.method, op.path, what, rr.Code, keys(op.codes))
	}
}

// keys returns the sorted keys of m.
func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pathMethods returns the documented methods of each path.
func pathMethods(ops []specOp) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, op := range ops {
		if out[op.path] == nil {
			out[op.path] = map[string]bool{}
		}
		out[op.path][op.method] = true
	}
	return out
}

// TestOpenAPIRoutesMatchRouter checks every documented operation is routed
// (a well-formed request is not answered 405, and not 404 where the spec
// documents no 404), that its status is documented, and that the methods the
// spec does not list get 405 with an Allow header listing exactly the
// documented methods.
func TestOpenAPIRoutesMatchRouter(t *testing.T) {
	ops := loadSpec(t)
	router := driftHandler(t, nil).Router()
	for _, op := range ops {
		t.Run(op.method+" "+op.path, func(t *testing.T) {
			rr := serve(router, driftRequest(op, op.method))
			if rr.Code == http.StatusMethodNotAllowed {
				t.Errorf("documented operation answered 405 (Allow %q)", rr.Header().Get("Allow"))
			}
			if _, has404 := op.codes["404"]; !has404 && rr.Code == http.StatusNotFound {
				t.Errorf("documented operation answered an undocumented 404: %s", rr.Body.String())
			}
			requireDocumented(t, op, rr, "a well-formed request")
		})
	}
	for path, documented := range pathMethods(ops) {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			if documented[method] {
				continue
			}
			t.Run("405 "+method+" "+path, func(t *testing.T) {
				rr := serve(router, driftRequest(specOp{path: path}, method))
				if strings.HasPrefix(path, "/i18n/") {
					// The spec says only GET is routed: anything else falls through to the 404 page.
					if rr.Code != http.StatusNotFound {
						t.Fatalf("undocumented method answered %d, want the 404 fallback", rr.Code)
					}
					return
				}
				if rr.Code != http.StatusMethodNotAllowed {
					t.Fatalf("undocumented method answered %d, want 405", rr.Code)
				}
				assertAllow(t, path, documented, rr.Header().Get("Allow"))
				for _, op := range ops {
					if op.path == path {
						requireDocumented(t, op, rr, method)
						if !strings.Contains(op.codes["405"], "Allow") {
							t.Errorf("the spec's 405 for %s does not document the Allow header", path)
						}
						return // the 405 is documented per path, so one operation is enough
					}
				}
			})
		}
	}
}

// assertAllow compares an Allow header with the documented methods. Probes
// and the catalog route also answer HEAD, which the spec leaves implicit.
func assertAllow(t *testing.T, path string, documented map[string]bool, allow string) {
	t.Helper()
	want := map[string]bool{}
	for m := range documented {
		want[m] = true
	}
	if !strings.HasPrefix(path, "/api/") {
		want[http.MethodHead] = true
	}
	got := map[string]bool{}
	for _, m := range strings.Split(allow, ",") {
		if m = strings.TrimSpace(m); m != "" {
			got[m] = true
		}
	}
	if len(got) != len(want) {
		t.Errorf("Allow = %q, documented methods %v", allow, want)
		return
	}
	for m := range want {
		if !got[m] {
			t.Errorf("Allow = %q, missing %s", allow, m)
		}
	}
}

// errService is a service whose every call fails with err, for provoking the
// handlers' error mapping with well-formed requests.
type errService struct{ err error }

func (s errService) CreateSecret(_ context.Context, ct io.Reader, _ int64, _ uint8, _ string, _ time.Duration) (app.Created, error) {
	_, _ = io.Copy(io.Discard, ct)
	return app.Created{}, s.err
}
func (s errService) Claim(context.Context, string, string) (app.ClaimResult, error) {
	return app.ClaimResult{}, s.err
}
func (s errService) Ack(context.Context, string, string) error { return s.err }
func (s errService) Status(context.Context, string, string) (app.SecretStatus, error) {
	return app.SecretStatus{}, s.err
}
func (s errService) Revoke(context.Context, string, string) error { return s.err }
func (s errService) CreateRequest(context.Context, time.Duration) (app.CreatedRequest, error) {
	return app.CreatedRequest{}, s.err
}
func (s errService) RequestOpen(context.Context, string, string) (time.Time, error) {
	return time.Time{}, s.err
}
func (s errService) Fill(context.Context, string, string, io.Reader, int64, uint8, string) (time.Time, error) {
	return time.Time{}, s.err
}
func (s errService) RequestStatus(context.Context, string, string) (app.RequestStatus, error) {
	return app.RequestStatus{}, s.err
}
func (s errService) ClaimReply(context.Context, string, string, string) (app.ClaimResult, error) {
	return app.ClaimResult{}, s.err
}
func (s errService) AckReply(context.Context, string, string) error { return s.err }
func (s errService) CancelRequest(context.Context, string, string) error {
	return s.err
}

// denyAll is a rate limiter that refuses every client.
type denyAll struct{}

func (denyAll) Allow(netip.Prefix) (bool, time.Duration) { return false, 3 * time.Second }

// apiOps filters the spec to the /api operations.
func apiOps(ops []specOp) []specOp {
	var out []specOp
	for _, op := range ops {
		if strings.HasPrefix(op.path, "/api/") {
			out = append(out, op)
		}
	}
	return out
}

// errorHandler is a router whose service fails with err.
func errorHandler(err error) http.Handler {
	svc := errService{err: err}
	h := httpx.New(svc, config.DefaultAppConfig.MaxBytes, nil)
	h.Requests = svc
	return h.Router()
}

// readOnlyOps cannot report 503 busy: they never take the write lock.
var readOnlyOps = map[string]bool{"GET /api/request/{id}": true, "GET /api/request/{id}/status": true}

// TestOpenAPIErrorStatuses provokes the error classes the spec lists and
// checks each answer is documented for the operation, with the headers the
// spec promises (Retry-After on 429 and 503, Allow on 405).
func TestOpenAPIErrorStatuses(t *testing.T) {
	ops := loadSpec(t)
	cases := []struct {
		name    string
		handler func() http.Handler
		want    int
		header  string
		skip    func(specOp) bool
	}{
		{"service busy", func() http.Handler { return errorHandler(app.ErrBusy) }, 503, "Retry-After",
			func(op specOp) bool { return readOnlyOps[op.method+" "+op.path] }},
		{"unexpected service error", func() http.Handler { return errorHandler(errors.New("boom")) }, 500, "", nil},
		{"rate limited", func() http.Handler {
			svc := errService{}
			h := httpx.New(svc, config.DefaultAppConfig.MaxBytes, nil)
			h.Requests, h.CreateLimiter, h.ReadLimiter = svc, denyAll{}, denyAll{}
			return h.Router()
		}, 429, "Retry-After", nil},
	}
	for _, tc := range cases {
		handler := tc.handler()
		for _, op := range apiOps(ops) {
			if tc.skip != nil && tc.skip(op) {
				continue
			}
			t.Run(tc.name+" "+op.method+" "+op.path, func(t *testing.T) {
				rr := serve(handler, driftRequest(op, op.method))
				if rr.Code != tc.want {
					t.Fatalf("got %d %s, want %d", rr.Code, rr.Body.String(), tc.want)
				}
				requireDocumented(t, op, rr, tc.name)
				if tc.header != "" {
					if rr.Header().Get(tc.header) == "" {
						t.Errorf("response lacks %s", tc.header)
					}
					if !strings.Contains(op.codes[strconv.Itoa(tc.want)], tc.header) {
						t.Errorf("spec does not document %s on %d", tc.header, tc.want)
					}
				}
			})
		}
	}
}

// TestOpenAPIClientErrors covers validation failures of the create call and
// bad IDs on every ID route.
func TestOpenAPIClientErrors(t *testing.T) {
	ops := loadSpec(t)
	router := driftHandler(t, nil).Router()
	find := func(method, path string) specOp {
		for _, op := range ops {
			if op.method == method && op.path == path {
				return op
			}
		}
		t.Fatalf("%s %s not documented", method, path)
		return specOp{}
	}
	create := find("POST", "/api/secret")
	mutations := map[string]struct {
		mutate func(*http.Request)
		want   int
	}{
		"missing content length": {func(r *http.Request) { r.Header.Del("Content-Length"); r.ContentLength = -1 }, 411},
		"invalid content length": {func(r *http.Request) { r.Header.Set("Content-Length", "abc") }, 400},
		"oversize":               {func(r *http.Request) { r.Header.Set("Content-Length", "999999999999") }, 413},
		"missing header":         {func(r *http.Request) { r.Header.Del("X-Gone-TTL") }, 400},
		"repeated header":        {func(r *http.Request) { r.Header.Add("X-Gone-Nonce", driftNonce) }, 400},
		"bad version":            {func(r *http.Request) { r.Header.Set("X-Gone-Version", "9") }, 400},
		"short body":             {func(r *http.Request) { r.Header.Set("Content-Length", "10") }, 400},
	}
	for name, m := range mutations {
		t.Run("create "+name, func(t *testing.T) {
			req := driftRequest(create, http.MethodPost)
			m.mutate(req)
			rr := serve(router, req)
			if rr.Code != m.want {
				t.Fatalf("got %d %s, want %d", rr.Code, rr.Body.String(), m.want)
			}
			requireDocumented(t, create, rr, name)
			if msg := strings.TrimSuffix(strings.SplitN(rr.Body.String(), `"`, 5)[3], `"`); !strings.Contains(create.codes[strconv.Itoa(rr.Code)], msg) && rr.Code == 400 {
				t.Errorf("the spec's 400 for create does not mention the error %q", msg)
			}
		})
	}
	for _, op := range apiOps(ops) {
		if !strings.Contains(op.path, "{id}") {
			continue
		}
		t.Run("bad id "+op.method+" "+op.path, func(t *testing.T) {
			req := driftRequest(op, op.method)
			req.URL.Path = strings.Replace(req.URL.Path, driftID, "not-an-id", 1)
			rr := serve(router, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("got %d %s, want 400", rr.Code, rr.Body.String())
			}
			requireDocumented(t, op, rr, "bad id")
		})
	}
}

// TestOpenAPIReadiness covers the probe's two bodies: text on 200, JSON on 503.
func TestOpenAPIReadiness(t *testing.T) {
	ops := loadSpec(t)
	var ready specOp
	for _, op := range ops {
		if op.path == "/readyz" {
			ready = op
		}
	}
	failing := driftHandler(t, func(context.Context) error { return errors.New("down") }).Router()
	rr := serve(failing, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("503 probe: %d %q", rr.Code, rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(ready.codes["503"], "application/json") || !strings.Contains(ready.codes["200"], "text/plain") {
		t.Fatalf("spec content types for /readyz: 200=%q 503=%q", ready.codes["200"], ready.codes["503"])
	}
}
