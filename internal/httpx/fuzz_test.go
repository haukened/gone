package httpx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/domain"
)

// headerVectors loads the shared server header vectors that seed the fuzzers.
func headerVectors(tb testing.TB) (versions, nonces []string) {
	tb.Helper()
	raw, err := os.ReadFile("../../test/vectors/server_headers.json")
	if err != nil {
		tb.Fatal(err)
	}
	var v struct {
		Versions []struct{ In string } `json:"versions"`
		Nonces   []struct{ In string } `json:"nonces"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		tb.Fatal(err)
	}
	for _, e := range v.Versions {
		versions = append(versions, e.In)
	}
	for _, e := range v.Nonces {
		nonces = append(nonces, e.In)
	}
	return versions, nonces
}

// reqWithHeaders builds a request whose headers hold the given values; an
// empty slice leaves the header absent, several values repeat it.
func reqWithHeaders(vals map[string][]string) *http.Request {
	r := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/api/secret"}, Header: http.Header{}}
	for k, v := range vals {
		r.Header[http.CanonicalHeaderKey(k)] = v
	}
	return r
}

// FuzzParseSecretHeaders checks parseSecretHeaders never panics, that every
// accepted input round-trips through the domain parsers, and that a repeated
// header is always rejected.
func FuzzParseSecretHeaders(f *testing.F) {
	versions, nonces := headerVectors(f)
	for _, v := range versions {
		for _, n := range nonces {
			f.Add(v, n, "5m", uint8(0))
		}
	}
	f.Add("1", "AAAAAAAAAAAAAAAA", "1h30m", uint8(1))
	f.Add("2", "AAAAAAAAAAAAAAAA", "-5m", uint8(2))
	f.Add("1", "AAAAAAAAAAAAAAAA", "", uint8(3))
	f.Fuzz(func(t *testing.T, version, nonce, ttl string, dup uint8) {
		vals := map[string][]string{"X-Gone-Version": {version}, "X-Gone-Nonce": {nonce}, "X-Gone-TTL": {ttl}}
		names := []string{"X-Gone-Version", "X-Gone-Nonce", "X-Gone-TTL"}
		repeated := dup%4 != 0
		if repeated {
			name := names[int(dup%4)-1]
			vals[name] = append(vals[name], vals[name][0])
		}
		v, n, d, err := parseSecretHeaders(reqWithHeaders(vals))
		if err != nil {
			return
		}
		if repeated {
			t.Fatalf("accepted a repeated header: %v", vals)
		}
		if pv, perr := domain.ParseVersion(version); perr != nil || pv != v {
			t.Fatalf("accepted version %q -> %d does not round-trip (%d, %v)", version, v, pv, perr)
		}
		if n != nonce || domain.ValidateProtocol(v, n) != nil {
			t.Fatalf("accepted nonce %q fails validation", nonce)
		}
		if pd, perr := time.ParseDuration(ttl); perr != nil || pd != d {
			t.Fatalf("accepted ttl %q -> %v does not round-trip", ttl, d)
		}
	})
}

// FuzzParseFillHeaders checks parseFillHeaders never panics and that accepted
// replies satisfy the reply protocol and the size limit.
func FuzzParseFillHeaders(f *testing.F) {
	versions, nonces := headerVectors(f)
	for _, v := range versions {
		for _, n := range nonces {
			f.Add("100", v, n, uint8(0))
		}
	}
	for _, cl := range []string{"", "0", "-1", "1048576", "1048577", "9223372036854775808", " 5", "5 ", "+5", "0x10"} {
		f.Add(cl, "3", "AAAAAAAAAAAAAAAA", uint8(0))
	}
	f.Add("10", "3", "AAAAAAAAAAAAAAAA", uint8(1))
	f.Fuzz(func(t *testing.T, cl, version, nonce string, dup uint8) {
		vals := map[string][]string{"Content-Length": {cl}, "X-Gone-Version": {version}, "X-Gone-Nonce": {nonce}}
		repeated := dup%3 != 0
		if repeated {
			name := []string{"X-Gone-Version", "X-Gone-Nonce"}[int(dup%3)-1]
			vals[name] = append(vals[name], vals[name][0])
		}
		if cl == "" {
			delete(vals, "Content-Length")
		}
		h := &Handler{MaxBody: 1 << 20}
		gotCL, v, n, err := h.parseFillHeaders(reqWithHeaders(vals))
		if err != nil {
			return
		}
		if repeated {
			t.Fatalf("accepted a repeated header: %v", vals)
		}
		if pc, perr := strconv.ParseInt(cl, 10, 64); perr != nil || pc != gotCL || gotCL <= 0 || gotCL > h.MaxBody {
			t.Fatalf("accepted content-length %q -> %d", cl, gotCL)
		}
		if pv, perr := domain.ParseReplyVersion(version); perr != nil || pv != v {
			t.Fatalf("accepted version %q does not round-trip", version)
		}
		if n != nonce || domain.ValidateReplyProtocol(v, n) != nil {
			t.Fatalf("accepted nonce %q fails validation", nonce)
		}
	})
}

// idRecorder is a ServicePort that records the IDs the router hands it.
type idRecorder struct{ ids []string }

func (s *idRecorder) CreateSecret(context.Context, io.Reader, int64, uint8, string, time.Duration) (app.Created, error) {
	return app.Created{}, app.ErrNotFound
}

func (s *idRecorder) Claim(_ context.Context, id, _ string) (app.ClaimResult, error) {
	s.ids = append(s.ids, id)
	return app.ClaimResult{}, app.ErrNotFound
}

func (s *idRecorder) Ack(_ context.Context, id, _ string) error {
	s.ids = append(s.ids, id)
	return app.ErrNotFound
}

func (s *idRecorder) Status(_ context.Context, id, _ string) (app.SecretStatus, error) {
	s.ids = append(s.ids, id)
	return app.SecretStatus{}, app.ErrNotFound
}

func (s *idRecorder) Revoke(_ context.Context, id, _ string) error {
	s.ids = append(s.ids, id)
	return app.ErrNotFound
}

// FuzzConsumeRoute checks the {id}/{action} extraction used by
// handleConsumeSecret: it never panics, it only ever hands the service the ID
// segment of the path, and an ID the domain accepts survives unchanged.
func FuzzConsumeRoute(f *testing.F) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, p := range []string{
		"/api/secret/" + id, "/api/secret/" + id + "/", "/api/secret/" + id + "/status",
		"/api/secret/" + id + "/revoke", "/api/secret/" + id + "/status/extra", "/api/secret/",
		"/api/secret", "/api/secret//status", "/api/secret/%2e%2e/status", "/api/secret/" + strings.ToUpper(id),
		"/api/secret/" + id + "%00", "/api/secret/../" + id,
	} {
		f.Add(p, uint8(0))
	}
	f.Fuzz(func(t *testing.T, path string, m uint8) {
		route, ok := consumeSecretRouteFromPath(path)
		if ok {
			if strings.Contains(route.id, "/") {
				t.Fatalf("id %q contains a slash", route.id)
			}
			rebuilt := "/api/secret/" + route.id
			if route.action != "" {
				rebuilt += "/" + route.action
			}
			if rebuilt != path && rebuilt+"/" != path {
				t.Fatalf("route %+v does not rebuild %q", route, path)
			}
			if sid, err := domain.ParseID(route.id); err == nil && sid.String() != route.id {
				t.Fatalf("accepted id %q changed to %q", route.id, sid)
			}
		}
		methods := []string{http.MethodGet, http.MethodDelete, http.MethodPost, http.MethodPut}
		svc := &idRecorder{}
		h := &Handler{Service: svc, MaxBody: 1024}
		rr := httptest.NewRecorder()
		req := &http.Request{Method: methods[int(m)%len(methods)], URL: &url.URL{Path: path}, Header: http.Header{}}
		h.handleConsumeSecret(rr, req.WithContext(context.Background()))
		for _, got := range svc.ids {
			if !ok || got != route.id {
				t.Fatalf("service got id %q for path %q (route %+v ok=%v)", got, path, route, ok)
			}
		}
		if rr.Code < 200 || rr.Code > 599 {
			t.Fatalf("status %d", rr.Code)
		}
	})
}
