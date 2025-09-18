// Package domain errors.go contains sentinel errors
package domain

import "errors"

// Sentinel domain-level errors reused by higher layers.
var (
	ErrInvalidID  = errors.New("invalid secret id")
	ErrTTLInvalid = errors.New("ttl invalid")
)
