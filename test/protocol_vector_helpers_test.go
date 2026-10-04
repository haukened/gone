package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// loadHeaderVectors reads test/vectors/server_headers.json.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - map[string][]headerVector: cases keyed by "versions" and "nonces".
func loadHeaderVectors(t *testing.T) map[string][]headerVector {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("vectors", "server_headers.json"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var v map[string][]headerVector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}
	return v
}
