// Package config handles configuration settings for the application.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Config holds the configuration settings for the application.
type Config struct {
	Addr     string        `koanf:"addr" validate:"required,ip_port"`
	DataDir  string        `koanf:"data_dir" validate:"required,custom_path"`
	MaxBytes int64         `koanf:"max_bytes" validate:"required,gt=0"`
	MinTTL   time.Duration `koanf:"min_ttl" validate:"required,gt=0"`
	MaxTTL   time.Duration `koanf:"max_ttl" validate:"required,gt=0"`
}

// DefaultAppConfig provides the default app configuration values.
var DefaultAppConfig = Config{
	Addr:     ":8080",
	DataDir:  "/data",
	MaxBytes: 1024 * 1024, // 1 MiB
	MinTTL:   5 * time.Minute,
	MaxTTL:   24 * time.Hour,
}

// defaultLoader loads default configuration values into the provided Koanf instance
// using the structs provider and the DefaultAppConfig struct. It returns an error
// if loading fails.
var defaultLoader = func(k *koanf.Koanf) error {
	return k.Load(structs.Provider(DefaultAppConfig, "koanf"), nil)
}

// envLoader is a function that loads environment variables with the prefix "GONE_".
// It transforms the keys to lowercase and removes the prefix.
// and can be mocked in tests.
var envLoader = func(k *koanf.Koanf) error {
	// Load environment variables with prefix "GONE_" using lowercase keys; all values scalar.
	return k.Load(env.Provider(".", env.Opt{Prefix: "GONE_", TransformFunc: func(key, value string) (string, any) {
		key = strings.ToLower(strings.TrimPrefix(key, "GONE_"))
		return key, strings.TrimSpace(value)
	}}), nil)
}

// validIPPort validates whether the provided field value is a valid IP address and port combination.
// It expects the value to be parseable by net.Listen()
// Examples: ":8080", "127.0.0.1:8080"
func validIPPort(fl validator.FieldLevel) bool {
	addr := fl.Field().String()
	ip, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return false
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return false
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	return err == nil && portNum > 0 && portNum < 65536
}

// validDirNotExists checks that the provided value is a directory path, but does not ensure it exists.
// It disallows empty paths, ".", the root directory, and paths that traverse upwards (contain "..").
func validDirNotExists(fl validator.FieldLevel) bool {
	raw := fl.Field().String()
	if raw == "" {
		return false
	}
	cleaned := filepath.Clean(raw)
	if cleaned == "." || cleaned == string(os.PathSeparator) {
		return false
	}
	// Split into components and reject explicit parent traversals.
	for _, part := range strings.Split(cleaned, string(os.PathSeparator)) {
		if part == ".." {
			return false
		}
	}
	return true
}

// registerValidators registers custom validation functions with the provided validator instance.
var registerValidators = func(v *validator.Validate) error {
	err := v.RegisterValidation("ip_port", validIPPort)
	if err != nil {
		return err
	}
	return v.RegisterValidation("custom_path", validDirNotExists)
}

// Load loads the configuration by applying default values and overriding them
// with environment variables. It validates the final configuration and returns
// a Config instance or an error if validation fails.
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load default values using structs provider.
	err := defaultLoader(k)
	if err != nil {
		return nil, err
	}

	// Override with environment variables.
	if err = envLoader(k); err != nil {
		return nil, err
	}

	var cfg Config

	// Unmarshal the config
	if err = k.Unmarshal("", &cfg); err != nil {
		return nil, err
	}

	// Create a new validator instance
	validate := validator.New(validator.WithRequiredStructEnabled())

	// Register custom validators
	if err = registerValidators(validate); err != nil {
		return nil, err
	}

	// Validate the config
	if err = validate.Struct(&cfg); err != nil {
		return nil, err
	}

	// ensure that MinTTL is less than MaxTTL
	if cfg.MinTTL >= cfg.MaxTTL {
		return nil, fmt.Errorf("min_ttl must be less than max_ttl")
	}

	return &cfg, nil
}

// SQLiteDSN returns a fixed hardened SQLite DSN derived from DataDir.
// WAL mode, foreign keys, busy timeout, and FULL synchronous are enforced.
func (c *Config) SQLiteDSN() string {
	dbPath := filepath.Join(c.DataDir, "gone.db")
	return fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_synchronous=FULL", dbPath)
}
