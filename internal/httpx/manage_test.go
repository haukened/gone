package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haukened/gone/internal/app"
	"github.com/haukened/gone/internal/domain"
	"github.com/haukened/gone/internal/httpx"
)

const manageID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// mustManage returns a freshly generated manage token string.
func mustManage(t *testing.T) domain.ManageToken {
	t.Helper()
	tok, err := domain.NewManageToken()
	if err != nil {
		t.Fatalf("NewManageToken: %v", err)
	}
	return tok
}

// manageRequest builds a request against an /api/secret/{id}/{action} path
// carrying the given X-Gone-Manage header values.
func manageRequest(method, path string, tokens ...string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	for _, tok := range tokens {
		req.Header.Add(httpx.HeaderManage, tok)
	}
	return req
}

// errorBody decodes the JSON error message from a response.
func errorBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error body %q: %v", w.Body.String(), err)
	}
	return resp.Error
}

func TestHandleStatusPending(t *testing.T) {
	tok := mustManage(t)
	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	expires := created.Add(time.Hour)
	m := mockService{statusFn: func(_ context.Context, id, got string) (app.SecretStatus, error) {
		if id != manageID || got != tok.String() {
			t.Fatalf("unexpected args id=%q tok=%q", id, got)
		}
		return app.SecretStatus{CreatedAt: created, ExpiresAt: expires}, nil
	}}
	w := httptest.NewRecorder()
	httpx.New(m, 1024, nil).Router().ServeHTTP(w, manageRequest(http.MethodGet, "/api/secret/"+manageID+"/status", tok.String()))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control %q", cc)
	}
	want := `{"state":"pending","created_at":"2025-01-02T02:04:05Z","expires_at":"2025-01-02T03:04:05Z"}`
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestHandleRevokeSuccess(t *testing.T) {
	tok := mustManage(t)
	called := false
	m := mockService{revokeFn: func(_ context.Context, id, got string) error {
		called = id == manageID && got == tok.String()
		return nil
	}}
	w := httptest.NewRecorder()
	httpx.New(m, 1024, nil).Router().ServeHTTP(w, manageRequest(http.MethodPost, "/api/secret/"+manageID+"/revoke", tok.String()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("service not called with id and token")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("unexpected body %q", w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control %q", cc)
	}
}

func TestManageRoutingMethods(t *testing.T) {
	tok := mustManage(t).String()
	tests := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantAllow string
	}{
		{"status POST", http.MethodPost, "/api/secret/" + manageID + "/status", http.StatusMethodNotAllowed, "GET"},
		{"status DELETE", http.MethodDelete, "/api/secret/" + manageID + "/status", http.StatusMethodNotAllowed, "GET"},
		{"revoke GET", http.MethodGet, "/api/secret/" + manageID + "/revoke", http.StatusMethodNotAllowed, "POST"},
		{"revoke DELETE", http.MethodDelete, "/api/secret/" + manageID + "/revoke", http.StatusMethodNotAllowed, "POST"},
		{"secret PUT", http.MethodPut, "/api/secret/" + manageID, http.StatusMethodNotAllowed, "GET, DELETE"},
		{"unknown action", http.MethodGet, "/api/secret/" + manageID + "/other", http.StatusNotFound, ""},
		{"nested action", http.MethodGet, "/api/secret/" + manageID + "/status/x", http.StatusNotFound, ""},
		{"empty id", http.MethodGet, "/api/secret/", http.StatusNotFound, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := mockService{
				statusFn: func(context.Context, string, string) (app.SecretStatus, error) {
					t.Fatal("status reached")
					return app.SecretStatus{}, nil
				},
				revokeFn: func(context.Context, string, string) error {
					t.Fatal("revoke reached")
					return nil
				},
			}
			w := httptest.NewRecorder()
			httpx.New(m, 1024, nil).Router().ServeHTTP(w, manageRequest(tc.method, tc.path, tok))
			if w.Code != tc.wantCode {
				t.Fatalf("status=%d want %d", w.Code, tc.wantCode)
			}
			if got := w.Header().Get("Allow"); got != tc.wantAllow {
				t.Fatalf("Allow=%q want %q", got, tc.wantAllow)
			}
		})
	}
}

func TestManageHeaderHandling(t *testing.T) {
	tok := mustManage(t).String()
	tests := []struct {
		name    string
		tokens  []string
		wantTok string
	}{
		{"missing header", nil, ""},
		{"duplicate header", []string{tok, tok}, ""},
		{"single header", []string{tok}, tok},
	}
	for _, tc := range tests {
		for _, ep := range []struct{ method, action string }{{http.MethodGet, "status"}, {http.MethodPost, "revoke"}} {
			t.Run(tc.name+" "+ep.action, func(t *testing.T) {
				var seen string
				check := func(got string) error {
					seen = got
					if got == "" {
						return domain.ErrInvalidManage
					}
					return app.ErrNotFound
				}
				m := mockService{
					statusFn: func(_ context.Context, _ string, got string) (app.SecretStatus, error) {
						return app.SecretStatus{}, check(got)
					},
					revokeFn: func(_ context.Context, _ string, got string) error { return check(got) },
				}
				w := httptest.NewRecorder()
				httpx.New(m, 1024, nil).Router().ServeHTTP(w, manageRequest(ep.method, "/api/secret/"+manageID+"/"+ep.action, tc.tokens...))
				if seen != tc.wantTok {
					t.Fatalf("service saw token %q want %q", seen, tc.wantTok)
				}
				wantCode, wantMsg := http.StatusBadRequest, "invalid manage token"
				if tc.wantTok != "" {
					wantCode, wantMsg = http.StatusNotFound, "not found"
				}
				if w.Code != wantCode || errorBody(t, w) != wantMsg {
					t.Fatalf("got %d %q want %d %q", w.Code, w.Body.String(), wantCode, wantMsg)
				}
			})
		}
	}
}

func TestManageNotFoundIsUniform(t *testing.T) {
	tok := mustManage(t).String()
	tests := []struct {
		name string
		err  error
	}{
		{"wrong token", app.ErrNotFound},
		{"unknown id", app.ErrNotFound},
		{"wrapped", errors.Join(errors.New("ctx"), app.ErrNotFound)},
	}
	var bodies [][]byte
	for _, tc := range tests {
		for _, ep := range []struct{ method, action string }{{http.MethodGet, "status"}, {http.MethodPost, "revoke"}} {
			m := mockService{
				statusFn: func(context.Context, string, string) (app.SecretStatus, error) { return app.SecretStatus{}, tc.err },
				revokeFn: func(context.Context, string, string) error { return tc.err },
			}
			w := httptest.NewRecorder()
			httpx.New(m, 1024, nil).Router().ServeHTTP(w, manageRequest(ep.method, "/api/secret/"+manageID+"/"+ep.action, tok))
			if w.Code != http.StatusNotFound {
				t.Fatalf("%s %s status=%d", tc.name, ep.action, w.Code)
			}
			bodies = append(bodies, w.Body.Bytes())
		}
	}
	for i := 1; i < len(bodies); i++ {
		if !bytes.Equal(bodies[0], bodies[i]) {
			t.Fatalf("404 bodies differ: %q vs %q", bodies[0], bodies[i])
		}
	}
}

func TestManageServiceErrors(t *testing.T) {
	tok := mustManage(t).String()
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"invalid id", domain.ErrInvalidID, http.StatusBadRequest},
		{"invalid manage", domain.ErrInvalidManage, http.StatusBadRequest},
		{"internal", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := mockService{
				statusFn: func(context.Context, string, string) (app.SecretStatus, error) { return app.SecretStatus{}, tc.err },
				revokeFn: func(context.Context, string, string) error { return tc.err },
			}
			router := httpx.New(m, 1024, nil).Router()
			for _, ep := range []struct{ method, action string }{{http.MethodGet, "status"}, {http.MethodPost, "revoke"}} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, manageRequest(ep.method, "/api/secret/"+manageID+"/"+ep.action, tok))
				if w.Code != tc.wantCode {
					t.Fatalf("%s status=%d want %d", ep.action, w.Code, tc.wantCode)
				}
			}
		})
	}
}

// pageTemplate is a SecretRenderer that writes a fixed body.
type pageTemplate struct{ body string }

// Execute implements httpx.SecretRenderer.
func (p pageTemplate) Execute(w http.ResponseWriter, _ any) error {
	_, err := w.Write([]byte(p.body))
	return err
}

func TestHandleManagePage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		tmpl     httpx.SecretRenderer
		wantCode int
		wantBody string
	}{
		{"renders page", "/manage/" + manageID, pageTemplate{body: "manage-page"}, http.StatusOK, "manage-page"},
		{"bare prefix", "/manage/", pageTemplate{body: "manage-page"}, http.StatusNotFound, "not found"},
		{"nil template", "/manage/" + manageID, nil, http.StatusServiceUnavailable, "manage template unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := httpx.New(mockService{}, 1024, nil)
			h.ManageTmpl = tc.tmpl
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("status=%d want %d", w.Code, tc.wantCode)
			}
			if !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("body %q missing %q", w.Body.String(), tc.wantBody)
			}
		})
	}
}
