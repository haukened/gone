package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goneapp "github.com/haukened/gone/v3/internal/app"
	gonecfg "github.com/haukened/gone/v3/internal/config"
	"github.com/haukened/gone/v3/internal/httpx"
	"github.com/haukened/gone/v3/internal/store"
	"github.com/haukened/gone/v3/internal/store/filesystem"
	"github.com/haukened/gone/v3/internal/store/sqlite"
)

// testEnv is a fake process environment with captured output.
type testEnv struct {
	*Env
	stdin     *bytes.Buffer
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
	vars      map[string]string
	passwords []string
	configDir string
}

// newTestEnv returns a non-interactive environment with an empty config
// directory.
//
// Parameters:
//   - t: the test.
//
// Returns the environment.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	te := &testEnv{
		stdin:     &bytes.Buffer{},
		stdout:    &bytes.Buffer{},
		stderr:    &bytes.Buffer{},
		vars:      map[string]string{},
		configDir: t.TempDir(),
	}
	te.Env = &Env{
		Stdin:     te.stdin,
		Stdout:    te.stdout,
		Stderr:    te.stderr,
		Getenv:    func(k string) string { return te.vars[k] },
		ConfigDir: func() (string, error) { return te.configDir, nil },
		Version:   "test",
		ReadPassword: func() ([]byte, error) {
			if len(te.passwords) == 0 {
				return nil, errors.New("no more passwords")
			}
			p := te.passwords[0]
			te.passwords = te.passwords[1:]
			return []byte(p), nil
		},
	}
	return te
}

// run executes the CLI with args and returns its exit code.
//
// Parameters:
//   - args: command-line arguments.
//
// Returns the exit code.
func (te *testEnv) run(args ...string) int {
	return Run(context.Background(), args, te.Env)
}

// reset clears captured output and queued input.
func (te *testEnv) reset() {
	te.stdin.Reset()
	te.stdout.Reset()
	te.stderr.Reset()
	te.passwords = nil
}

// realClock is the wall clock used by the test server.
type realClock struct{}

// Now returns the current UTC time.
//
// Returns the time.
func (realClock) Now() time.Time { return time.Now().UTC() }

// newGoneServer starts the real Gone HTTP stack over TLS and points te at
// its certificate.
//
// Parameters:
//   - t: the test.
//   - te: environment whose RootCAs are set to trust the server.
//
// Returns the server.
func newGoneServer(t *testing.T, te *testEnv) *httptest.Server {
	t.Helper()
	return newTLS(t, te, newGoneHandler(t))
}

// newGoneHandler builds the real Gone router over a temporary SQLite index
// and blob directory.
//
// Parameters:
//   - t: the test.
//
// Returns the router.
func newGoneHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := gonecfg.DefaultAppConfig
	dir := t.TempDir()
	db, err := sql.Open(sqlite.DriverName, gonecfg.SQLiteDSNFor(dir))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	idx, err := sqlite.New(db)
	if err != nil {
		t.Fatalf("init index: %v", err)
	}
	blobDir := filepath.Join(dir, "blobs")
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	blobs, err := filesystem.New(blobDir)
	if err != nil {
		t.Fatalf("blobs: %v", err)
	}
	svc := &goneapp.Service{
		Store:      store.New(idx, blobs, realClock{}, cfg.InlineMaxBytes),
		Clock:      realClock{},
		MaxBytes:   cfg.MaxBytes,
		MinTTL:     cfg.MinTTL,
		MaxTTL:     cfg.MaxTTL,
		ClaimLease: cfg.ClaimLease,
	}
	svc.Requests = svc.Store.(goneapp.RequestStore)
	h := httpx.New(svc, cfg.MaxBytes, nil)
	h.MinTTL, h.MaxTTL = cfg.MinTTL, cfg.MaxTTL
	h.Requests = svc
	return h.Router()
}

// newTLS starts a TLS test server for h and points te at its certificate.
//
// Parameters:
//   - t: the test.
//   - te: environment whose RootCAs are set to trust the server.
//   - h: the handler.
//
// Returns the server.
func newTLS(t *testing.T, te *testEnv, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	te.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	return srv
}

// linkLine extracts the value of a "Label: value" line from text output.
//
// Parameters:
//   - t: the test.
//   - out: command output.
//   - label: line label including the colon.
//
// Returns the trimmed value.
func linkLine(t *testing.T, out, label string) string {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if v, ok := strings.CutPrefix(line, label); ok {
			return strings.TrimSpace(v)
		}
	}
	t.Fatalf("no %q line in %q", label, out)
	return ""
}

// errWriter is a writer that always fails.
type errWriter struct{}

// Write fails.
//
// Parameters:
//   - p: ignored.
//
// Returns 0 and an error.
func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("write failed") }

// errReader is a reader that always fails.
type errReader struct{}

// Read fails.
//
// Parameters:
//   - p: ignored.
//
// Returns 0 and an error.
func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("read failed") }

// writeTemp writes data to a new file in a temporary directory.
//
// Parameters:
//   - t: the test.
//   - name: file name.
//   - data: contents.
//
// Returns the file path.
func writeTemp(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
