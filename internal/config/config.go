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
	"github.com/go-viper/mapstructure/v2"
	"github.com/haukened/gone/internal/domain"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Config holds the configuration settings for the application.
type Config struct {
	Addr           string             `koanf:"addr" validate:"required,ip_port"`
	DataDir        string             `koanf:"data_dir" validate:"required,custom_path"`
	InlineMaxBytes int64              `koanf:"inline_max_bytes" validate:"required,gt=0"`
	MaxBytes       int64              `koanf:"max_bytes" validate:"required,gt=0"`
	MinTTL         time.Duration      `koanf:"-" validate:"required,ltfield=MaxTTL"`
	MaxTTL         time.Duration      `koanf:"-" validate:"required,gtfield=MinTTL"`
	TTLOptions     []domain.TTLOption `koanf:"ttl_options" validate:"required"`
	MetricsAddr    string             `koanf:"metrics_addr" validate:"omitempty,ip_port"`
	MetricsToken   string             `koanf:"metrics_token"`
}

// DefaultAppConfig provides the default app configuration values.
var DefaultAppConfig = Config{
	Addr:           ":8080",
	DataDir:        "/data",
	InlineMaxBytes: 8192,        // 8 KiB
	MaxBytes:       1024 * 1024, // 1 MiB
	MinTTL:         5 * time.Minute,
	MaxTTL:         24 * time.Hour,
	TTLOptions: []domain.TTLOption{
		{
			Duration: 5 * time.Minute,
			Label:    "5m",
		},
		{
			Duration: 30 * time.Minute,
			Label:    "30m",
		},
		{
			Duration: 1 * time.Hour,
			Label:    "1h",
		},
		{
			Duration: 2 * time.Hour,
			Label:    "2h",
		},
		{
			Duration: 4 * time.Hour,
			Label:    "4h",
		},
		{
			Duration: 8 * time.Hour,
			Label:    "8h",
		},
		{
			Duration: 24 * time.Hour,
			Label:    "24h",
		},
	},
	MetricsAddr: "", // disabled by default
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
		if strings.Contains(value, ",") {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return key, parts
		}
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
	err = k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			Result:           &cfg,
			TagName:          "koanf",
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				StringToTTLOptions(),
			),
		},
	})
	if err != nil {
		return nil, err
	}

	// Create a new validator instance
	validate := validator.New(validator.WithRequiredStructEnabled())

	// Register custom validators
	if err = registerValidators(validate); err != nil {
		return nil, err
	}

	// Calculate the MinTTL and MaxTTL from TTLOptions
	// koanf ensures TTLOptions is always non-nil
	for _, opt := range cfg.TTLOptions {
		if cfg.MinTTL == 0 || opt.Duration < cfg.MinTTL {
			cfg.MinTTL = opt.Duration
		}
		if opt.Duration > cfg.MaxTTL {
			cfg.MaxTTL = opt.Duration
		}
	}

	// Validate the config
	if err = validate.Struct(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// SQLiteDSN returns a fixed hardened SQLite DSN derived from DataDir.
// WAL mode, foreign keys, busy timeout, and FULL synchronous are enforced.
func (c *Config) SQLiteDSN() string {
	dbPath := filepath.Join(c.DataDir, "gone.db")
	return fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_synchronous=FULL", dbPath)
}
