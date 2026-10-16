package httpx_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/httpx"
)

const routeID = "0123456789abcdef0123456789abcdef"

// TestBarePagePrefixesRenderHTML404 checks an ID-less page route answers with
// the HTML error page, not the API's JSON error.
func TestBarePagePrefixesRenderHTML404(t *testing.T) {
	h := httpx.New(mockService{}, 1024, nil)
	h.ErrorTmpl = httpx.TemplateRenderer{T: template.Must(template.New("e").Parse("<p>{{ .Status }}</p>"))}
	h.SecretTmpl, h.ManageTmpl, h.ReplyTmpl = pageTemplate{}, pageTemplate{}, pageTemplate{}
	h.RequestDetailTmpl = pageTemplate{}
	for _, p := range []string{"/secret/", "/manage/", "/reply/", "/request/"} {
		w := httptest.NewRecorder()
		h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusNotFound || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || w.Body.String() != "<p>404</p>" {
			t.Errorf("%s: %d %q %q", p, w.Code, w.Header().Get("Content-Type"), w.Body.String())
		}
	}
}

// TestTrailingSlashIsNotARoute checks that a trailing slash after an ID or
// action never aliases the route without it.
func TestTrailingSlashIsNotARoute(t *testing.T) {
	h := httpx.New(mockService{}, 1024, nil)
	h.Requests = &fakeRequests{}
	for _, p := range []string{
		"/api/secret/" + routeID + "/", "/api/secret/" + routeID + "/status/", "/api/secret/" + routeID + "/revoke/",
		"/api/request/" + routeID + "/", "/api/request/" + routeID + "/reply/", "/api/request/" + routeID + "/status/",
	} {
		for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, httptest.NewRequest(m, p, nil))
			if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"not found"`) {
				t.Errorf("%s %s: %d %s, want 404", m, p, w.Code, w.Body.String())
			}
		}
	}
}

// TestProbeMethods checks the probes answer GET and HEAD and refuse the rest
// with an Allow header.
func TestProbeMethods(t *testing.T) {
	h := httpx.New(mockService{}, 1024, nil).Router()
	for _, p := range []string{"/healthz", "/readyz"} {
		for _, m := range []string{http.MethodGet, http.MethodHead} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(m, p, nil))
			if w.Code != http.StatusOK {
				t.Errorf("%s %s = %d", m, p, w.Code)
			}
		}
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(m, p, nil))
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
				t.Errorf("%s %s = %d allow=%q", m, p, w.Code, w.Header().Get("Allow"))
			}
		}
	}
}

// TestCreateMethodNotAllowedHasAllow checks the 405 on /api/secret names POST.
func TestCreateMethodNotAllowedHasAllow(t *testing.T) {
	w := httptest.NewRecorder()
	httpx.New(mockService{}, 1024, nil).Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/secret", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "POST" {
		t.Fatalf("got %d allow=%q", w.Code, w.Header().Get("Allow"))
	}
}
