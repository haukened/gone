package httpx

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docCSP returns the Content-Security-Policy value listed in the "Security
// headers" section of docs/security.md.
func docCSP(t *testing.T) string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "docs", "security.md"))
	if err != nil {
		t.Fatalf("open docs/security.md: %v", err)
	}
	defer func() { _ = f.Close() }()
	const prefix = "- `Content-Security-Policy: "
	inSection := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			inSection = line == "## Security headers"
			continue
		}
		if inSection && strings.HasPrefix(line, prefix) && strings.HasSuffix(line, "`") {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), "`")
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read docs/security.md: %v", err)
	}
	t.Fatal("no Content-Security-Policy line in the Security headers section of docs/security.md")
	return ""
}

// TestDocumentedCSPMatchesMiddleware fails when docs/security.md and the
// header the middleware sets drift apart.
func TestDocumentedCSPMatchesMiddleware(t *testing.T) {
	h := &Handler{}
	rw := httptest.NewRecorder()
	h.secureHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/", nil))
	got := rw.Result().Header.Get("Content-Security-Policy")
	if want := docCSP(t); got != want {
		t.Fatalf("docs/security.md CSP differs from the middleware\ndoc:        %s\nmiddleware: %s", want, got)
	}
}
