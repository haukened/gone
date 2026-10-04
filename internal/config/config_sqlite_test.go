package config

import (
	"strings"
	"testing"
)

func TestSQLiteDSN(t *testing.T) {
	cleanGoneEnvForTest(t)
	for _, tt := range sqliteDSNTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Addr: ":8080", DataDir: tt.dataDir, MaxBytes: DefaultAppConfig.MaxBytes}
			want := "file:" + joinSQLiteTestPath(tt.dataDir, "gone.db") + sqliteTestParams
			got := c.SQLiteDSN()
			if got != want {
				t.Fatalf("SQLiteDSN() = %q, want %q", got, want)
			}
			assertSQLiteDSNPragmas(t, got)
		})
	}
}

const sqliteTestParams = "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)"

type sqliteDSNCase struct {
	name    string
	dataDir string
}

// sqliteDSNTestCases returns data directory cases for SQLite DSN tests.
func sqliteDSNTestCases() []sqliteDSNCase {
	return []sqliteDSNCase{
		{name: "default_config", dataDir: DefaultAppConfig.DataDir},
		{name: "relative_no_slash", dataDir: "data"},
		{name: "relative_trailing_slash", dataDir: "data/"},
		{name: "absolute_no_slash", dataDir: "/var/lib/gone"},
		{name: "absolute_trailing_slash", dataDir: "/var/lib/gone/"},
	}
}

// joinSQLiteTestPath joins two path fragments using the production DSN rules.
func joinSQLiteTestPath(a, b string) string {
	if len(a) == 0 {
		return b
	}
	if a[len(a)-1] == '/' {
		return a + b
	}
	return a + "/" + b
}

// assertSQLiteDSNPragmas verifies all hardened pragmas are present once.
func assertSQLiteDSNPragmas(t *testing.T, got string) {
	t.Helper()
	for _, want := range []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
		"_pragma=synchronous(FULL)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %q", want, got)
		}
	}
	if strings.Count(got, "?") != 1 {
		t.Fatalf("expected exactly one '?' in DSN, got %q", got)
	}
}
