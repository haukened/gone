package config

import "testing"

func TestValidPaths(t *testing.T) {
	cleanGoneEnvForTest(t)
	valid := []string{"data", "/var/lib/gone", "./data", "relative/path/to/data", "nested/dir/structure"}
	for _, p := range valid {
		t.Run(p, func(t *testing.T) {
			t.Setenv("GONE_DATA_DIR", p)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("expected valid path %q, got error: %v", p, err)
			}
			if cfg.DataDir != p {
				t.Fatalf("expected DataDir %q, got %q", p, cfg.DataDir)
			}
		})
	}
}

func TestInvalidPaths(t *testing.T) {
	cleanGoneEnvForTest(t)
	invalid := []string{"", ".", "/", "//", "../data", "data/..", "data/../../../etc"}
	for _, p := range invalid {
		t.Run(p, func(t *testing.T) {
			t.Setenv("GONE_DATA_DIR", p)
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for invalid path %q, got nil", p)
			}
		})
	}
}
