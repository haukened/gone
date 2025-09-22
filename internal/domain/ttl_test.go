package domain

import (
	"strings"
	"testing"
	"time"
)

// TestNewTTLOptionValid verifies that valid TTL labels are parsed correctly and
// that the returned TTLOption contains the expected Duration and normalized Label.
func TestNewTTLOptionValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantDur   time.Duration
		wantLabel string
	}{
		{
			name:      "minutes",
			input:     "5m",
			wantDur:   5 * time.Minute,
			wantLabel: "5m",
		},
		{
			name:      "compound hours and minutes",
			input:     "1h30m",
			wantDur:   time.Hour + 30*time.Minute,
			wantLabel: "1h30m",
		},
		{
			name:      "hours boundary 24h",
			input:     "24h",
			wantDur:   24 * time.Hour,
			wantLabel: "24h",
		},
		{
			name:      "trim surrounding whitespace",
			input:     " 10s ",
			wantDur:   10 * time.Second,
			wantLabel: "10s",
		},
		{
			name:      "seconds",
			input:     "45s",
			wantDur:   45 * time.Second,
			wantLabel: "45s",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opt, err := NewTTLOption(tc.input)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if opt.Duration != tc.wantDur {
				t.Fatalf("expected duration %v, got %v", tc.wantDur, opt.Duration)
			}
			if opt.Label != tc.wantLabel {
				t.Fatalf("expected label %q, got %q", tc.wantLabel, opt.Label)
			}
		})
	}
}

// TestNewTTLOptionInvalid verifies that invalid labels produce appropriate errors.
func TestNewTTLOptionInvalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr string // substring expected in error
	}{
		{
			name:    "empty string",
			input:   "",
			wantErr: "empty TTL label",
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: "empty TTL label",
		},
		{
			name:    "unsupported day unit",
			input:   "1d",
			wantErr: "unsupported TTL unit",
		},
		{
			name:    "unsupported week unit",
			input:   "2w",
			wantErr: "unsupported TTL unit",
		},
		{
			name:    "unsupported month unit uppercase M",
			input:   "5M",
			wantErr: "unsupported TTL unit",
		},
		{
			name:    "unsupported year unit",
			input:   "1y",
			wantErr: "unsupported TTL unit",
		},
		{
			name:    "nonsense format",
			input:   "abc",
			wantErr: "time: invalid duration", // from time.ParseDuration
		},
		{
			name:    "bad unit",
			input:   "10q",
			wantErr: "time: unknown unit", // from time.ParseDuration
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTTLOption(tc.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestNewTTLOptionDoesNotAllowUnsupportedUnitsEmbedded ensures any presence of unsupported unit letters causes rejection.
func TestNewTTLOptionDoesNotAllowUnsupportedUnitsEmbedded(t *testing.T) {
	t.Parallel()
	input := "1day" // contains 'd'
	_, err := NewTTLOption(input)
	if err == nil {
		t.Fatalf("expected error for input %q, got nil", input)
	}
	if !strings.Contains(err.Error(), "unsupported TTL unit") {
		t.Fatalf("expected unsupported unit error, got %v", err)
	}
}
