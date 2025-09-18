package domain

import (
	"testing"
	"time"
)

func TestValidateTTL(t *testing.T) {
	minTTL := time.Minute
	maxTTL := 10 * time.Minute
	if err := ValidateTTL(5*time.Minute, minTTL, maxTTL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateTTL(0, minTTL, maxTTL); err == nil {
		t.Errorf("expected error for zero")
	}
	if err := ValidateTTL(30*time.Second, minTTL, maxTTL); err == nil {
		t.Errorf("expected error for below min")
	}
	if err := ValidateTTL(11*time.Minute, minTTL, maxTTL); err == nil {
		t.Errorf("expected error for above max")
	}
}

func TestClampTTL(t *testing.T) {
	minTTL := time.Minute
	maxTTL := 10 * time.Minute
	if got := ClampTTL(30*time.Second, minTTL, maxTTL); got != minTTL {
		t.Errorf("expected clamp to min, got %v", got)
	}
	if got := ClampTTL(15*time.Minute, minTTL, maxTTL); got != maxTTL {
		t.Errorf("expected clamp to max, got %v", got)
	}
	mid := 5 * time.Minute
	if got := ClampTTL(mid, minTTL, maxTTL); got != mid {
		t.Errorf("expected passthrough, got %v", got)
	}
}

func TestIsTTLValid(t *testing.T) {
	minTTL := time.Minute
	maxTTL := 10 * time.Minute
	if !IsTTLValid(5*time.Minute, minTTL, maxTTL) {
		t.Errorf("expected ttl valid")
	}
	if IsTTLValid(0, minTTL, maxTTL) {
		t.Errorf("expected invalid (zero)")
	}
}
