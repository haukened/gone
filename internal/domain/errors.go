// Package domain errors.go contains sentinel errors
package domain

import "errors"

// Sentinel domain-level errors reused by higher layers.
var (
	ErrInvalidID  = errors.New("invalid secret id")
	ErrTTLInvalid = errors.New("ttl invalid")
	// ErrInvalidClaim indicates a malformed claim token.
	ErrInvalidClaim = errors.New("invalid claim token")
	// ErrInvalidManage indicates a malformed sender manage token.
	ErrInvalidManage = errors.New("invalid manage token")
	// ErrInvalidFill indicates a malformed request fill token.
	ErrInvalidFill = errors.New("invalid fill token")
	// ErrInvalidVersion indicates a malformed or unsupported protocol version.
	ErrInvalidVersion = errors.New("invalid version")
	// ErrInvalidNonce indicates a nonce that is not strict base64url of the
	// version's nonce size.
	ErrInvalidNonce = errors.New("invalid nonce")
	// ErrInvalidB64 indicates input that is not strict, canonical, unpadded
	// base64url.
	ErrInvalidB64 = errors.New("invalid base64url")
)
