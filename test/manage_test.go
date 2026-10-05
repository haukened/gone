package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/haukened/gone/v3/internal/ratelimit"
)

func TestManageStatusThenRevoke(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)

	resp := manageReq(t, http.MethodGet, srv.URL, id, token)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	requireManageStatusOK(t, resp)
	requireManageStatusCode(t, "revoke", revoke(t, srv.URL, id, token), http.StatusNoContent)
	requireManageStatusCode(t, "claim after revoke", readSecretStatus(t, srv.URL, id, ""), http.StatusNotFound)
	requireManageStatusCode(t, "status after revoke", status(t, srv.URL, id, token), http.StatusNotFound)
	requireManageStatusCode(t, "second revoke", revoke(t, srv.URL, id, token), http.StatusNotFound)
}

func TestManageStatusAcrossClaimAndAck(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)

	claim := claimToken(t, srv.URL, id)
	if got := status(t, srv.URL, id, token); got != http.StatusOK {
		t.Fatalf("status during lease = %d, want 200", got)
	}
	if got := ack(t, srv.URL, id, claim); got != http.StatusNoContent {
		t.Fatalf("ack = %d, want 204", got)
	}
	if got := status(t, srv.URL, id, token); got != http.StatusNotFound {
		t.Fatalf("status after ack = %d, want 404", got)
	}
}

func TestManageRevokeWinsDuringLease(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)

	claim := claimToken(t, srv.URL, id)
	if got := revoke(t, srv.URL, id, token); got != http.StatusNoContent {
		t.Fatalf("revoke during lease = %d, want 204", got)
	}
	if got := ack(t, srv.URL, id, claim); got != http.StatusNotFound {
		t.Fatalf("ack after revoke = %d, want 404", got)
	}
	if got := readSecretStatus(t, srv.URL, id, claim); got != http.StatusNotFound {
		t.Fatalf("re-read after revoke = %d, want 404", got)
	}
}

func TestManageRejectsWrongOrMissingToken(t *testing.T) {
	srv := newServer(t, &fakeClock{t: time.Unix(1_700_000_000, 0)}, limits{})
	id, token := createManaged(t, srv.URL)
	wrong := otherToken(token)

	cases := []struct {
		name   string
		method string
		token  string
		want   int
	}{
		{"status missing", http.MethodGet, "", http.StatusBadRequest},
		{"status malformed", http.MethodGet, "short", http.StatusBadRequest},
		{"status wrong", http.MethodGet, wrong, http.StatusNotFound},
		{"revoke missing", http.MethodPost, "", http.StatusBadRequest},
		{"revoke wrong", http.MethodPost, wrong, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := manageReqStatus(t, tc.method, srv.URL, id, tc.token); got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}

	// None of the rejected attempts may have touched the secret.
	if got := status(t, srv.URL, id, token); got != http.StatusOK {
		t.Fatalf("status with real token = %d, want 200", got)
	}
	claim := claimToken(t, srv.URL, id)
	if got := ack(t, srv.URL, id, claim); got != http.StatusNoContent {
		t.Fatalf("ack = %d, want 204", got)
	}
}

func TestManageSharesReadBudget(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	srv := newServer(t, clk, limits{read: ratelimit.Rate{Count: 2, Per: time.Hour}, burst: 2})
	id, token := createManaged(t, srv.URL)

	if got := status(t, srv.URL, id, token); got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	if got := readSecretStatus(t, srv.URL, id, ""); got != http.StatusOK {
		t.Fatalf("claim = %d, want 200", got)
	}
	resp := manageReq(t, http.MethodPost, srv.URL, id, token)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	assertLimited(t, resp)
}
