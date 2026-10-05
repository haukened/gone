package config

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/haukened/gone/v3/internal/ratelimit"
)

// Minimum prefix lengths accepted for trusted proxies. Broader ranges would
// let large parts of the internet spoof X-Forwarded-For.
const (
	minTrustedBitsV4 = 8
	minTrustedBitsV6 = 32
)

// deriveRateLimits parses the raw rate-limit settings into their typed forms.
//
// Parameters:
//   - cfg: configuration whose RateCreate, RateRead, and TrustedProxies are
//     read and whose CreateRate, ReadRate, and TrustedPrefixes are written.
//
// Returns:
//   - error: a parse error naming the offending environment variable.
func deriveRateLimits(cfg *Config) error {
	var err error
	if cfg.CreateRate, err = ratelimit.ParseRate(cfg.RateCreate); err != nil {
		return fmt.Errorf("GONE_RATE_CREATE: %w", err)
	}
	if cfg.ReadRate, err = ratelimit.ParseRate(cfg.RateRead); err != nil {
		return fmt.Errorf("GONE_RATE_READ: %w", err)
	}
	if cfg.TrustedPrefixes, err = parseTrustedProxies(cfg.TrustedProxies); err != nil {
		return fmt.Errorf("GONE_TRUSTED_PROXIES: %w", err)
	}
	return nil
}

// parseTrustedProxies converts trusted proxy entries into masked prefixes.
// Blank entries are skipped and a bare address is treated as a single host.
//
// Parameters:
//   - entries: CIDR prefixes or bare IP addresses.
//
// Returns:
//   - []netip.Prefix: the masked prefixes; nil when there are none.
//   - error: when an entry is malformed, too broad, or IPv4-mapped IPv6.
func parseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		p, err := parseTrustedProxy(entry)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// parseTrustedProxy parses and validates a single trusted proxy entry.
//
// Parameters:
//   - entry: a non-blank CIDR prefix or bare IP address.
//
// Returns:
//   - netip.Prefix: the masked prefix.
//   - error: when the entry is malformed, too broad, or IPv4-mapped IPv6.
func parseTrustedProxy(entry string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(entry)
	if err != nil {
		addr, aerr := netip.ParseAddr(entry)
		if aerr != nil {
			return netip.Prefix{}, fmt.Errorf("invalid entry %q: want CIDR or IP", entry)
		}
		p = netip.PrefixFrom(addr, addr.BitLen())
	}
	if p.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("invalid entry %q: use plain IPv4 notation", entry)
	}
	minBits := minTrustedBitsV6
	if p.Addr().Is4() {
		minBits = minTrustedBitsV4
	}
	if p.Bits() < minBits {
		return netip.Prefix{}, fmt.Errorf("invalid entry %q: prefix broader than /%d", entry, minBits)
	}
	return p.Masked(), nil
}
