package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/config"
)

// TestPages_Policy checks the compose pages follow the operator policy: no
// file controls when files are off, and a passphrase field that is always
// shown and required on the send page. Replies are exempt from the
// passphrase rule.
func TestPages_Policy(t *testing.T) {
	h := pageRouterWith(t, func(c *config.Config) {
		c.RequirePassphrase = true
		c.MaxFiles = 0
	})
	cases := []struct {
		path    string
		want    []string
		notWant []string
	}{
		{"/", []string{`id="pass-required"`, `data-pass-required`, `data-max-files="0"`, ` required aria-required="true"`},
			[]string{`id="drop-zone"`, `id="secret-files"`, `<details id="pass-disclosure"`}},
		{"/reply/" + strings.Repeat("d", 32), []string{`data-max-files="0"`},
			[]string{`id="drop-zone"`, `id="secret-files"`, `data-pass-required`}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			checkPageContract(t, h, pageContractCase{tc.path, http.StatusOK, []string{"create-secret", "size-box"}}, map[string]bool{})
			_, body := get(h, tc.path)
			for _, s := range tc.want {
				if !strings.Contains(body, s) {
					t.Errorf("missing %q", s)
				}
			}
			for _, s := range tc.notWant {
				if strings.Contains(body, s) {
					t.Errorf("unexpected %q", s)
				}
			}
		})
	}
}

// TestPages_FileLimitLabel checks the files label shows the configured limit.
func TestPages_FileLimitLabel(t *testing.T) {
	h := pageRouterWith(t, func(c *config.Config) { c.MaxFiles = 3 })
	_, body := get(h, "/")
	if !strings.Contains(body, `data-max-files="3"`) || !strings.Contains(body, `&#34;max&#34;:3`) {
		t.Fatalf("files limit 3 not rendered")
	}
}
