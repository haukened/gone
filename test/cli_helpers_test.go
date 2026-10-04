package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haukened/gone/internal/cli"
)

// cliRunner drives the gone CLI in-process against a test server.
type cliRunner struct {
	env                   *cli.Env
	stdin, stdout, stderr *bytes.Buffer
}

// newTLSServer serves h over TLS and closes it when the test ends.
//
// Parameters:
//   - t: the test.
//   - h: the handler to serve.
//
// Returns:
//   - *httptest.Server: the running server.
func newTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// newCLI returns a runner that trusts srv's certificate and has an empty
// config directory and environment.
//
// Parameters:
//   - t: the test.
//   - srv: the TLS server the CLI talks to.
//
// Returns:
//   - *cliRunner: the runner.
func newCLI(t *testing.T, srv *httptest.Server) *cliRunner {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	cfg := t.TempDir()
	r := &cliRunner{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	r.env = &cli.Env{
		Stdin:        r.stdin,
		Stdout:       r.stdout,
		Stderr:       r.stderr,
		ReadPassword: func() ([]byte, error) { return nil, errors.New("no terminal") },
		Getenv:       func(string) string { return "" },
		ConfigDir:    func() (string, error) { return cfg, nil },
		Version:      "integration",
		RootCAs:      pool,
	}
	return r
}

// run executes the CLI with stdin set to input and returns the exit code.
// Previous output is discarded first.
//
// Parameters:
//   - input: data supplied on stdin.
//   - args: command-line arguments.
//
// Returns:
//   - int: the exit code.
func (r *cliRunner) run(input string, args ...string) int {
	r.stdin.Reset()
	r.stdout.Reset()
	r.stderr.Reset()
	r.stdin.WriteString(input)
	return cli.Run(context.Background(), args, r.env)
}

// send runs "gone send" and returns the share and manage links, failing
// the test on a non-zero exit.
//
// Parameters:
//   - t: the test.
//   - input: the message.
//   - args: extra send arguments, including --server.
//
// Returns:
//   - link: the share link.
//   - manage: the manage link.
func (r *cliRunner) send(t *testing.T, input string, args ...string) (link, manage string) {
	t.Helper()
	if code := r.run(input, append([]string{"send"}, args...)...); code != 0 {
		t.Fatalf("send exit %d: %s", code, r.stderr)
	}
	return r.field(t, "Link:"), r.field(t, "Manage link:")
}

// field returns the value after prefix on a line of stdout.
//
// Parameters:
//   - t: the test.
//   - prefix: the line label, such as "Link:".
//
// Returns:
//   - string: the trimmed value.
func (r *cliRunner) field(t *testing.T, prefix string) string {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(r.stdout.String()))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	t.Fatalf("no %q line in %q", prefix, r.stdout)
	return ""
}

// writeFile creates a file named name containing data in a new temporary
// directory.
//
// Parameters:
//   - t: the test.
//   - name: the base file name.
//   - data: the contents.
//
// Returns:
//   - string: the file path.
func writeFile(t *testing.T, name, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
