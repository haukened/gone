package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxClaimLease is the longest claim lease accepted from GONE_CLAIM_LEASE.
const maxClaimLease = 15 * time.Minute

// validate checks a fully loaded configuration, after MinTTL, MaxTTL, and the
// rate limits have been derived.
//
// Parameters:
//   - cfg: the configuration to check.
//
// Returns:
//   - error: the first invalid setting, named by its environment variable.
func validate(cfg *Config) error {
	for _, check := range []func(*Config) error{validatePaths, validateSizes, validateTTLs, validateRateBurst} {
		if err := check(cfg); err != nil {
			return err
		}
	}
	return nil
}

// validatePaths checks the listen addresses and the data directory.
//
// Parameters:
//   - cfg: the configuration to check.
//
// Returns:
//   - error: naming GONE_ADDR, GONE_METRICS_ADDR, or GONE_DATA_DIR when invalid.
func validatePaths(cfg *Config) error {
	switch {
	case !validIPPort(cfg.Addr):
		return fmt.Errorf("GONE_ADDR: %q is not [ip]:port with a port from 1 to 65535", cfg.Addr)
	case cfg.MetricsAddr != "" && !validIPPort(cfg.MetricsAddr):
		return fmt.Errorf("GONE_METRICS_ADDR: %q is not [ip]:port with a port from 1 to 65535", cfg.MetricsAddr)
	case !validDataDir(cfg.DataDir):
		return fmt.Errorf("GONE_DATA_DIR: %q is not allowed; use a directory other than \".\" or \"/\" with no \"..\" part", cfg.DataDir)
	}
	return nil
}

// validateSizes checks the size limits.
//
// Parameters:
//   - cfg: the configuration to check.
//
// Returns:
//   - error: naming GONE_INLINE_MAX_BYTES or GONE_MAX_BYTES when not positive.
func validateSizes(cfg *Config) error {
	if cfg.InlineMaxBytes <= 0 {
		return fmt.Errorf("GONE_INLINE_MAX_BYTES: must be greater than 0, got %d", cfg.InlineMaxBytes)
	}
	if cfg.MaxBytes <= 0 {
		return fmt.Errorf("GONE_MAX_BYTES: must be greater than 0, got %d", cfg.MaxBytes)
	}
	return nil
}

// validateTTLs checks the TTL choices and the claim lease.
//
// Parameters:
//   - cfg: the configuration to check.
//
// Returns:
//   - error: naming GONE_TTL_OPTIONS or GONE_CLAIM_LEASE when invalid.
func validateTTLs(cfg *Config) error {
	switch {
	case len(cfg.TTLOptions) == 0:
		return errors.New("GONE_TTL_OPTIONS: at least one TTL is required")
	case cfg.MinTTL <= 0 || cfg.MinTTL >= cfg.MaxTTL:
		return fmt.Errorf("GONE_TTL_OPTIONS: need at least two different positive TTLs, got %s to %s", cfg.MinTTL, cfg.MaxTTL)
	case cfg.ClaimLease <= 0 || cfg.ClaimLease > maxClaimLease:
		return fmt.Errorf("GONE_CLAIM_LEASE: must be greater than 0 and at most %s, got %s", maxClaimLease, cfg.ClaimLease)
	}
	return nil
}

// validateRateBurst checks the rate limiter burst size.
//
// Parameters:
//   - cfg: the configuration to check.
//
// Returns:
//   - error: naming GONE_RATE_BURST when outside 1-1000.
func validateRateBurst(cfg *Config) error {
	if cfg.RateBurst < 1 || cfg.RateBurst > 1000 {
		return fmt.Errorf("GONE_RATE_BURST: must be from 1 to 1000, got %d", cfg.RateBurst)
	}
	return nil
}

// validIPPort reports whether addr is an optional IP address and a port, as
// accepted by net.Listen. Examples: ":8080", "127.0.0.1:8080", "[::1]:8080".
//
// Parameters:
//   - addr: the address to check.
//
// Returns:
//   - bool: true when the host is empty or an IP and the port is 1-65535.
func validIPPort(addr string) bool {
	ip, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return false
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return false
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	return err == nil && portNum > 0
}

// validDataDir reports whether raw is usable as a data directory path. The
// directory need not exist. Empty paths, ".", the root directory, and paths
// that contain a ".." component are rejected.
//
// Parameters:
//   - raw: the path to check.
//
// Returns:
//   - bool: true when the path is acceptable.
func validDataDir(raw string) bool {
	if raw == "" {
		return false
	}
	cleaned := filepath.Clean(raw)
	if cleaned == "." || cleaned == string(os.PathSeparator) {
		return false
	}
	for _, part := range strings.Split(cleaned, string(os.PathSeparator)) {
		if part == ".." {
			return false
		}
	}
	return true
}
