// Package domain ttl.go contains functions to validate TTL against config values.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type TTLOption struct {
	Duration time.Duration
	Label    string // human-friendly label for UI
}

// NewTTLOption parses a duration string and returns a TTLOption.
// It returns an error if parsing fails.
// supports standard time.Duration strings like "5m", "1h30m", "24h"
// Supported units:
//
//	s - seconds
//	m - minutes
//	h - hours
func NewTTLOption(label string) (TTLOption, error) {
	// reject empty or whitespace-only labels
	label = strings.TrimSpace(label)
	if label == "" {
		return TTLOption{}, errors.New("empty TTL label")
	}
	// reject unsupported units (e.g., days, weeks)
	if strings.ContainsAny(label, "dwMy") {
		return TTLOption{}, fmt.Errorf("unsupported TTL unit in %q", label)
	}
	d, err := time.ParseDuration(label)
	if err != nil {
		return TTLOption{}, err
	}
	return TTLOption{Duration: d, Label: label}, nil
}
