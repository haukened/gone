// Package config provides layered configuration loading for the Gone service.
// It merges Defaults -> Environment Variables -> CLI Flags, with validation.
package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

/*
Usage example (in main package):

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fv := config.BindFlags(fs)
	_ = fs.Parse(os.Args[1:])

	cfg := config.Defaults()
	_ = config.FromEnv(&cfg, config.GetenvPresent)
	_ = config.ApplyFlags(&cfg, fv)
	if errs := config.Validate(cfg); len(errs) > 0 {
		for _, e := range errs { fmt.Fprintf(os.Stderr, "config error: %v\n", e) }
		os.Exit(1)
	}
*/

// Config holds the merged runtime configuration for the Gone service.
// Order of precedence (lowest → highest): Defaults → Environment → CLI Flags.
type Config struct {
	Addr     string        // listen address, e.g. ":8080"
	DataDir  string        // directory for blobs, e.g. "./data"
	DSN      string        // SQLite DSN, e.g. "file:gone.db?_journal_mode=WAL&_fk=1"
	MaxBytes int64         // max POST body size (bytes), e.g. 128 KiB
	MinTTL   time.Duration // minimum TTL allowed
	MaxTTL   time.Duration // maximum TTL allowed
}

// Defaults returns a Config populated with secure, minimal sane defaults.
func Defaults() Config {
	return Config{
		Addr:     ":8080",
		DataDir:  "./data",
		DSN:      "file:gone.db?_journal_mode=WAL&_fk=1",
		MaxBytes: 128 << 10, // 128 KiB
		MinTTL:   1 * time.Minute,
		MaxTTL:   7 * 24 * time.Hour, // 7 days
	}
}

// GetenvPresent wraps os.LookupEnv to satisfy the required getenv func signature.
func GetenvPresent(name string) (string, bool) { return os.LookupEnv(name) }

// FromEnv overlays environment variables onto the provided Config. Missing vars are ignored.
// Parsing errors are wrapped with the environment variable name for clarity.
func FromEnv(cfg *Config, getenv func(string) (string, bool)) error {
	if cfg == nil {
		return fmt.Errorf("nil config passed to FromEnv")
	}

	// Simple string mappings (no parsing logic required)
	strVars := []struct {
		env string
		dst *string
	}{
		{"GONE_ADDR", &cfg.Addr},
		{"GONE_DATA_DIR", &cfg.DataDir},
		{"GONE_DSN", &cfg.DSN},
	}
	for _, sv := range strVars {
		if v, ok := getenv(sv.env); ok {
			*sv.dst = v
		}
	}

	// Parsers for numeric/duration values with empty-string clearing semantics.
	type parseFn func(raw string) error
	parsers := []struct {
		env string
		fn  parseFn
	}{
		{"GONE_MAX_BYTES", func(raw string) error {
			if raw == "" {
				cfg.MaxBytes = 0
				return nil
			}
			n, err := ParseSize(raw)
			if err != nil {
				return err
			}
			cfg.MaxBytes = n
			return nil
		}},
		{"GONE_MIN_TTL", func(raw string) error {
			if raw == "" {
				cfg.MinTTL = 0
				return nil
			}
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			cfg.MinTTL = d
			return nil
		}},
		{"GONE_MAX_TTL", func(raw string) error {
			if raw == "" {
				cfg.MaxTTL = 0
				return nil
			}
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			cfg.MaxTTL = d
			return nil
		}},
	}
	for _, p := range parsers {
		if v, ok := getenv(p.env); ok {
			if err := p.fn(v); err != nil {
				return fmt.Errorf("%s: %w", p.env, err)
			}
		}
	}
	return nil
}

// FlagVals holds raw string representations of flag values so emptiness can signal "unset".
type FlagVals struct {
	Addr     string
	DataDir  string
	DSN      string
	MaxBytes string
	MinTTL   string
	MaxTTL   string
}

// BindFlags defines the configuration flags on the provided FlagSet and returns a FlagVals
// capturing their string pointers. All defaults are empty to distinguish from "not provided".
func BindFlags(fs *flag.FlagSet) *FlagVals {
	fv := &FlagVals{}
	fs.StringVar(&fv.Addr, "addr", "", "Listen address (e.g. :8080)")
	fs.StringVar(&fv.DataDir, "data-dir", "", "Directory for encrypted blobs")
	fs.StringVar(&fv.DSN, "dsn", "", "SQLite DSN (e.g. file:gone.db?_journal_mode=WAL&_fk=1)")
	fs.StringVar(&fv.MaxBytes, "max-bytes", "", "Maximum secret size (bytes or IEC, e.g. 128KiB)")
	fs.StringVar(&fv.MinTTL, "min-ttl", "", "Minimum TTL (e.g. 1m, 30s)")
	fs.StringVar(&fv.MaxTTL, "max-ttl", "", "Maximum TTL (e.g. 168h, 24h)")
	return fv
}

// ApplyFlags overlays non-empty flag values onto the provided Config, parsing as needed.
// Parsing errors are wrapped with the flag name.
func ApplyFlags(cfg *Config, fv *FlagVals) error {
	if cfg == nil {
		return fmt.Errorf("nil config passed to ApplyFlags")
	}
	if fv == nil {
		return nil
	}

	// Direct string assignments (unset => empty string ignored)
	strFlags := []struct {
		val string
		dst *string
	}{
		{fv.Addr, &cfg.Addr},
		{fv.DataDir, &cfg.DataDir},
		{fv.DSN, &cfg.DSN},
	}
	for _, f := range strFlags {
		if f.val != "" {
			*f.dst = f.val
		}
	}

	// Parsers with flag-specific error wrapping labels
	type parseFn func(raw string) error
	parsers := []struct {
		raw   string
		label string
		fn    parseFn
	}{
		{fv.MaxBytes, "-max-bytes", func(raw string) error {
			n, err := ParseSize(raw)
			if err != nil {
				return err
			}
			cfg.MaxBytes = n
			return nil
		}},
		{fv.MinTTL, "-min-ttl", func(raw string) error {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			cfg.MinTTL = d
			return nil
		}},
		{fv.MaxTTL, "-max-ttl", func(raw string) error {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			cfg.MaxTTL = d
			return nil
		}},
	}
	for _, p := range parsers {
		if p.raw == "" {
			continue
		}
		if err := p.fn(p.raw); err != nil {
			return fmt.Errorf("%s: %w", p.label, err)
		}
	}
	return nil
}

// Validate performs logical checks and returns all encountered problems.
func Validate(cfg Config) []error {
	var errs []error
	if cfg.Addr == "" {
		errs = append(errs, fmt.Errorf("addr must not be empty"))
	}
	if cfg.DataDir == "" {
		errs = append(errs, fmt.Errorf("data dir must not be empty"))
	}
	if cfg.MaxBytes <= 0 {
		errs = append(errs, fmt.Errorf("max bytes must be > 0"))
	}
	if cfg.MinTTL <= 0 {
		errs = append(errs, fmt.Errorf("min ttl must be > 0"))
	}
	if cfg.MaxTTL < cfg.MinTTL {
		errs = append(errs, fmt.Errorf("max ttl must be >= min ttl"))
	}
	return errs
}

// ParseSize converts a human-friendly size string into a byte count.
// Accepts plain integers (bytes) or IEC/human suffixes: KiB/MiB/GiB (case-insensitive) or K/M/G.
// Examples: "131072" => 131072, "128KiB" => 131072, "1MiB" => 1048576, "2G" => 2147483648.
func ParseSize(s string) (int64, error) {
	orig := s
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	upper := strings.ToUpper(s)
	// Attempt suffix parsing first.
	if n, ok, err := parseSizeWithSuffix(upper, orig); ok {
		return n, err
	}
	// Fallback: plain integer bytes.
	n, err := parsePositiveInt(upper)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", orig, err)
	}
	return n, nil
}

// parsePositiveInt parses a base-10 int64 and rejects negatives.
func parsePositiveInt(raw string) (int64, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("negative not allowed")
	}
	return n, nil
}

// parseSizeWithSuffix attempts to parse well-known size suffixes. It returns (value, true, nil)
// on success; (0, false, nil) if no suffix matched; or (0, true, error) if a suffix matched but parsing failed.
func parseSizeWithSuffix(upper, orig string) (int64, bool, error) {
	type unit struct {
		suffix string
		mult   int64
	}
	units := []unit{
		{"KIB", 1024}, {"MIB", 1024 * 1024}, {"GIB", 1024 * 1024 * 1024},
		{"K", 1024}, {"M", 1024 * 1024}, {"G", 1024 * 1024 * 1024},
	}
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			numPart := strings.TrimSpace(upper[:len(upper)-len(u.suffix)])
			if numPart == "" {
				return 0, true, fmt.Errorf("parse size %q: missing number", orig)
			}
			n, err := parsePositiveInt(numPart)
			if err != nil {
				return 0, true, fmt.Errorf("parse size %q: %w", orig, err)
			}
			return n * u.mult, true, nil
		}
	}
	return 0, false, nil
}
