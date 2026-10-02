// Package domain errors.go contains sentinel errors
package domain

import "errors"

// Sentinel domain-level errors reused by higher layers.
var (
	ErrInvalidID  = errors.New("invalid secret id")
	ErrTTLInvalid = errors.New("ttl invalid")
	// ErrInvalidClaim indicates a malformed claim token.
	ErrInvalidClaim = errors.New("invalid claim token")
)
