// Package ratelimit implements per-client token-bucket rate limiting for the
// Gone HTTP API. It is pure logic with no HTTP dependencies: callers derive a
// client key with ClientKey and ask a Limiter whether a request may proceed.
// State is held in memory per process and is bounded in size.
package ratelimit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxRateCount bounds the request count accepted in a rate specification.
const maxRateCount = 1_000_000

// ErrInvalidRate reports a malformed rate specification.
var ErrInvalidRate = errors.New("invalid rate")

// rateUnits maps the accepted unit suffixes to their durations.
var rateUnits = map[string]time.Duration{
	"s": time.Second,
	"m": time.Minute,
	"h": time.Hour,
}

// Rate is a request budget of Count requests per Per interval.
// The zero value means rate limiting is disabled.
type Rate struct {
	Count int
	Per   time.Duration
}

// Enabled reports whether the rate imposes a limit.
//
// Returns:
//   - bool: true when Count and Per are both positive.
func (r Rate) Enabled() bool {
	return r.Count > 0 && r.Per > 0
}

// PerSecond converts the rate into tokens per second.
//
// Returns:
//   - float64: tokens added per second; 0 when the rate is disabled.
func (r Rate) PerSecond() float64 {
	if !r.Enabled() {
		return 0
	}
	return float64(r.Count) / r.Per.Seconds()
}

// ParseRate parses a rate specification of the form "N/u", where N is a
// non-negative integer and u is one of "s", "m", or "h". A count of zero, or
// the bare string "0", disables the limit. Signs, decimals, and whitespace are
// rejected.
//
// Parameters:
//   - s: the specification, for example "10/m".
//
// Returns:
//   - Rate: the parsed rate; the zero value when disabled.
//   - error: ErrInvalidRate (wrapped) when s is malformed or out of range.
func ParseRate(s string) (Rate, error) {
	if s == "0" {
		return Rate{}, nil
	}
	countStr, unit, found := strings.Cut(s, "/")
	per, unitOK := rateUnits[unit]
	if !found || !unitOK {
		return Rate{}, fmt.Errorf("%w: want N/s, N/m, or N/h", ErrInvalidRate)
	}
	count, err := parseCount(countStr)
	if err != nil {
		return Rate{}, err
	}
	if count == 0 {
		return Rate{}, nil
	}
	return Rate{Count: count, Per: per}, nil
}

// parseCount parses the decimal request count of a rate specification.
//
// Parameters:
//   - s: digits only; at most maxRateCount.
//
// Returns:
//   - int: the parsed count.
//   - error: ErrInvalidRate (wrapped) when s is empty, non-numeric, or too large.
func parseCount(s string) (int, error) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, fmt.Errorf("%w: count must be digits", ErrInvalidRate)
	}
	n, err := strconv.Atoi(s)
	if err != nil || n > maxRateCount {
		return 0, fmt.Errorf("%w: count must be at most %d", ErrInvalidRate, maxRateCount)
	}
	return n, nil
}
