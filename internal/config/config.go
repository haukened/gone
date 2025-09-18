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

	if v, ok := getenv("GONE_ADDR"); ok {
		cfg.Addr = v
	}
	if v, ok := getenv("GONE_DATA_DIR"); ok {
		cfg.DataDir = v
	}
	if v, ok := getenv("GONE_DSN"); ok {
		cfg.DSN = v
	}
	if v, ok := getenv("GONE_MAX_BYTES"); ok {
		if v != "" { // allow empty to intentionally clear, though Validate will catch
			n, err := ParseSize(v)
			if err != nil {
				return fmt.Errorf("GONE_MAX_BYTES: %w", err)
			}
			cfg.MaxBytes = n
		} else {
			cfg.MaxBytes = 0
		}
	}
	if v, ok := getenv("GONE_MIN_TTL"); ok {
		if v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("GONE_MIN_TTL: %w", err)
			}
			cfg.MinTTL = d
		} else {
			cfg.MinTTL = 0
		}
	}
	if v, ok := getenv("GONE_MAX_TTL"); ok {
		if v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("GONE_MAX_TTL: %w", err)
			}
			cfg.MaxTTL = d
		} else {
			cfg.MaxTTL = 0
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
	if fv.Addr != "" {
		cfg.Addr = fv.Addr
	}
	if fv.DataDir != "" {
		cfg.DataDir = fv.DataDir
	}
	if fv.DSN != "" {
		cfg.DSN = fv.DSN
	}
	if fv.MaxBytes != "" {
		n, err := ParseSize(fv.MaxBytes)
		if err != nil {
			return fmt.Errorf("-max-bytes: %w", err)
		}
		cfg.MaxBytes = n
	}
	if fv.MinTTL != "" {
		d, err := time.ParseDuration(fv.MinTTL)
		if err != nil {
			return fmt.Errorf("-min-ttl: %w", err)
		}
		cfg.MinTTL = d
	}
	if fv.MaxTTL != "" {
		d, err := time.ParseDuration(fv.MaxTTL)
		if err != nil {
			return fmt.Errorf("-max-ttl: %w", err)
		}
		cfg.MaxTTL = d
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

	type unit struct {
		suffix string
		mult   int64
	}
	units := []unit{
		{"KIB", 1024},
		{"MIB", 1024 * 1024},
		{"GIB", 1024 * 1024 * 1024},
		{"K", 1024},
		{"M", 1024 * 1024},
		{"G", 1024 * 1024 * 1024},
	}
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			numPart := strings.TrimSpace(upper[:len(upper)-len(u.suffix)])
			if numPart == "" {
				return 0, fmt.Errorf("parse size %q: missing number", orig)
			}
			n, err := strconv.ParseInt(numPart, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse size %q: %w", orig, err)
			}
			if n < 0 {
				return 0, fmt.Errorf("parse size %q: negative not allowed", orig)
			}
			return n * u.mult, nil
		}
	}
	// No recognized suffix → treat as plain integer bytes.
	n, err := strconv.ParseInt(upper, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", orig, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("parse size %q: negative not allowed", orig)
	}
	return n, nil
}
