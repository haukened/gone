// Package domain ttl.go contains functions to validate TTL against config values.
package domain

import "time"

// ValidateTTL checks that ttl is positive and within [min, max].
// Returns ErrTTLInvalid on any violation.
func ValidateTTL(ttl, minTTL, maxTTL time.Duration) error {
	if ttl <= 0 {
		return ErrTTLInvalid
	}
	if ttl < minTTL {
		return ErrTTLInvalid
	}
	if ttl > maxTTL {
		return ErrTTLInvalid
	}
	return nil
}

// ClampTTL returns ttl constrained to the inclusive range [min, max].
// If ttl < min it returns min; if ttl > max it returns max; otherwise ttl.
func ClampTTL(ttl, minTTL, maxTTL time.Duration) time.Duration {
	if ttl < minTTL {
		return minTTL
	}
	if ttl > maxTTL {
		return maxTTL
	}
	return ttl
}

// IsTTLValid is a convenience wrapper returning true if ValidateTTL reports no error.
func IsTTLValid(ttl, minTTL, maxTTL time.Duration) bool {
	return ValidateTTL(ttl, minTTL, maxTTL) == nil
}
