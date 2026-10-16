package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/httpx"
)

// passRequiredService fails every create with ErrPassphraseRequired, as the
// service does for a v1 secret when the policy is on.
type passRequiredService struct{ failingService }

func (passRequiredService) CreateSecret(context.Context, io.Reader, int64, uint8, string, time.Duration) (app.Created, error) {
	return app.Created{}, app.ErrPassphraseRequired
}

func TestPolicyEndpoint(t *testing.T) {
	h := httpx.New(failingService{}, 1024, nil)
	h.RequirePassphrase, h.MaxFiles = true, 3
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/policy", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var body struct {
		PassphraseRequired bool `json:"passphrase_required"`
		MaxFiles           *int `json:"max_files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.PassphraseRequired || body.MaxFiles == nil || *body.MaxFiles != 3 {
		t.Fatalf("body = %s, want passphrase_required true and max_files 3", w.Body.String())
	}
}

func TestPolicyEndpointMethod(t *testing.T) {
	h := httpx.New(failingService{}, 1024, nil)
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/policy", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
		t.Fatalf("status = %d Allow = %q, want 405 GET", w.Code, w.Header().Get("Allow"))
	}
}

func TestCreateRequirePassphrase(t *testing.T) {
	cases := []struct {
		name     string
		required bool
		version  string
		service  httpx.ServicePort
		want     int
	}{
		{"v1 rejected", true, "1", failingService{}, http.StatusForbidden},
		{"v2 accepted", true, "2", failingService{}, http.StatusCreated},
		{"v1 accepted when off", false, "1", failingService{}, http.StatusCreated},
		{"service rejection mapped", false, "1", passRequiredService{}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/secret", bytes.NewReader(nil))
			req.Header.Set("Content-Length", "10")
			req.Header.Set("X-Gone-Version", tc.version)
			req.Header.Set("X-Gone-Nonce", "AAAAAAAAAAAAAAAA")
			req.Header.Set("X-Gone-TTL", "5m")
			h := httpx.New(tc.service, 1024, nil)
			h.RequirePassphrase = tc.required
			w := httptest.NewRecorder()
			h.Router().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == http.StatusForbidden && !bytes.Contains(w.Body.Bytes(), []byte("passphrase required")) {
				t.Fatalf("body = %s, want passphrase required", w.Body.String())
			}
		})
	}
}
