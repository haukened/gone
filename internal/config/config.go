// Package config handles configuration settings for the application.
package config

import (
	"net/netip"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/ratelimit"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Config holds the configuration settings for the application.
type Config struct {
	Addr           string             `koanf:"addr"`
	DataDir        string             `koanf:"data_dir"`
	InlineMaxBytes int64              `koanf:"inline_max_bytes"`
	MaxBytes       int64              `koanf:"max_bytes"`
	MinTTL         time.Duration      `koanf:"-"`
	MaxTTL         time.Duration      `koanf:"-"`
	TTLOptions     []domain.TTLOption `koanf:"ttl_options"`
	ClaimLease     time.Duration      `koanf:"claim_lease"`
	MetricsAddr    string             `koanf:"metrics_addr"`
	MetricsToken   string             `koanf:"metrics_token"`

	// Rate limiting: raw settings loaded from GONE_RATE_* and
	// GONE_TRUSTED_PROXIES, and the typed values derived from them.
	RateCreate      string         `koanf:"rate_create"`
	RateRead        string         `koanf:"rate_read"`
	RateBurst       int            `koanf:"rate_burst"`
	TrustedProxies  []string       `koanf:"trusted_proxies"`
	CreateRate      ratelimit.Rate `koanf:"-"`
	ReadRate        ratelimit.Rate `koanf:"-"`
	TrustedPrefixes []netip.Prefix `koanf:"-"`
}

// DefaultAppConfig provides the default app configuration values.
var DefaultAppConfig = Config{
	Addr:           ":8080",
	DataDir:        "/data",
	InlineMaxBytes: 8192,             // 8 KiB
	MaxBytes:       10 * 1024 * 1024, // 10 MiB (message + attachments combined)
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
	ClaimLease:  2 * time.Minute, // window to finish download + decrypt before a claim lapses
	MetricsAddr: "",              // disabled by default
	RateCreate:  "10/m",
	RateRead:    "30/m",
	RateBurst:   10,
	CreateRate:  ratelimit.Rate{Count: 10, Per: time.Minute},
	ReadRate:    ratelimit.Rate{Count: 30, Per: time.Minute},
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
				mapstructure.StringToTimeDurationHookFunc(),
			),
		},
	})
	if err != nil {
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

	if err = deriveRateLimits(&cfg); err != nil {
		return nil, err
	}

	if err = validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// MetricsEnabled reports whether the metrics listener should be started.
// Metrics require both a listen address and a bearer token; if either is
// missing, metrics are disabled so the endpoint is never exposed unauthenticated.
//
// Returns:
//   - bool: true when both MetricsAddr and MetricsToken are non-empty.
func (c *Config) MetricsEnabled() bool {
	return c.MetricsAddr != "" && c.MetricsToken != ""
}
