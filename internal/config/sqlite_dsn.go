package config

import (
	"fmt"
	"path/filepath"
)

// SQLiteDSN returns a fixed hardened SQLite DSN derived from DataDir.
// See SQLiteDSNFor for the enforced pragmas.
//
// Returns:
//   - string: a modernc.org/sqlite DSN for <DataDir>/gone.db.
func (c *Config) SQLiteDSN() string {
	return SQLiteDSNFor(c.DataDir)
}

// SQLiteDSNFor returns a hardened modernc.org/sqlite DSN for the gone.db file
// inside dataDir. The pragmas (WAL journal mode, foreign keys, busy timeout,
// and FULL synchronous) are applied by the driver to every new connection.
//
// Parameters:
//   - dataDir: directory that holds (or will hold) gone.db.
//
// Returns:
//   - string: DSN suitable for sql.Open("sqlite", dsn).
func SQLiteDSNFor(dataDir string) string {
	dbPath := filepath.Join(dataDir, "gone.db")
	return fmt.Sprintf("file:%s?%s", dbPath, sqlitePragmas)
}

// sqlitePragmas is the hardened pragma query string applied to every connection.
const sqlitePragmas = "_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)"
