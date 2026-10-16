package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/httpx"
)

// TestClaimHeader_DuplicateRejected checks a repeated X-Gone-Claim is a 400 on
// every claim and ack route, and never reaches the service (in particular, a
// duplicate never turns into a fresh claim).
func TestClaimHeader_DuplicateRejected(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	called := false
	svc := mockService{
		claimFn: func(context.Context, string, string) (app.ClaimResult, error) {
			called = true
			return app.ClaimResult{}, nil
		},
		ackFn: func(context.Context, string, string) error { called = true; return nil },
	}
	reqs := &fakeRequests{}
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/secret/" + id},
		{http.MethodDelete, "/api/secret/" + id},
		{http.MethodGet, "/api/request/" + id + "/reply"},
		{http.MethodDelete, "/api/request/" + id + "/reply"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			h := httpx.New(svc, 1024, nil)
			h.Requests = reqs
			req := httptest.NewRequest(rt.method, rt.path, nil)
			req.Header.Add(httpx.HeaderClaim, "a")
			req.Header.Add(httpx.HeaderClaim, "b")
			rr := httptest.NewRecorder()
			h.Router().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid claim") {
				t.Fatalf("got %d %s, want 400 invalid claim", rr.Code, rr.Body.String())
			}
			if called {
				t.Fatal("service was called with a duplicated claim header")
			}
		})
	}
}
