package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestPolicy(t *testing.T) {
	var method, path string
	c, _ := newTLS(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = io.WriteString(w, `{"passphrase_required":true,"max_files":0,"sign_in_required":true,"extra":1}`)
	})
	p, err := c.Policy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet || path != "/api/policy" {
		t.Fatalf("request = %s %s", method, path)
	}
	if p != (Policy{PassphraseRequired: true, MaxFiles: 0, SignInRequired: true}) {
		t.Fatalf("policy = %+v", p)
	}
}

func TestPolicyErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"old server", http.StatusNotFound, `{"error":"not found"}`, ErrNotFound},
		{"server", http.StatusInternalServerError, ``, ErrServer},
		{"bad json", http.StatusOK, `{`, ErrProtocol},
		{"missing field", http.StatusOK, `{"passphrase_required":false}`, ErrProtocol},
		{"negative files", http.StatusOK, `{"passphrase_required":false,"max_files":-1}`, ErrProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTLS(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			if _, err := c.Policy(context.Background()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
