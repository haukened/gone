package ratelimit

import (
	"errors"
	"testing"
	"time"
)

func TestParseRate(t *testing.T) {
	tests := []struct {
		in      string
		want    Rate
		wantErr bool
	}{
		{in: "10/m", want: Rate{Count: 10, Per: time.Minute}},
		{in: "1/s", want: Rate{Count: 1, Per: time.Second}},
		{in: "500/h", want: Rate{Count: 500, Per: time.Hour}},
		{in: "1000000/s", want: Rate{Count: 1_000_000, Per: time.Second}},
		{in: "0", want: Rate{}},
		{in: "0/m", want: Rate{}},
		{in: "", wantErr: true},
		{in: "10", wantErr: true},
		{in: "10/", wantErr: true},
		{in: "10/d", wantErr: true},
		{in: "10/M", wantErr: true},
		{in: "/m", wantErr: true},
		{in: "+5/m", wantErr: true},
		{in: "-5/m", wantErr: true},
		{in: "1.5/m", wantErr: true},
		{in: " 5/m", wantErr: true},
		{in: "5 /m", wantErr: true},
		{in: "5/m ", wantErr: true},
		{in: "1000001/s", wantErr: true},
		{in: "99999999999999999999999/s", wantErr: true},
		{in: "5/m/s", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseRate(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidRate) {
					t.Fatalf("ParseRate(%q) err = %v, want ErrInvalidRate", tc.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRate(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseRate(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRateEnabledAndPerSecond(t *testing.T) {
	tests := []struct {
		name    string
		rate    Rate
		enabled bool
		perSec  float64
	}{
		{name: "zero", rate: Rate{}, enabled: false, perSec: 0},
		{name: "no interval", rate: Rate{Count: 5}, enabled: false, perSec: 0},
		{name: "negative count", rate: Rate{Count: -1, Per: time.Second}, enabled: false, perSec: 0},
		{name: "per second", rate: Rate{Count: 4, Per: time.Second}, enabled: true, perSec: 4},
		{name: "per minute", rate: Rate{Count: 30, Per: time.Minute}, enabled: true, perSec: 0.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rate.Enabled(); got != tc.enabled {
				t.Fatalf("Enabled() = %v, want %v", got, tc.enabled)
			}
			if got := tc.rate.PerSecond(); got != tc.perSec {
				t.Fatalf("PerSecond() = %v, want %v", got, tc.perSec)
			}
		})
	}
}

func FuzzParseRate(f *testing.F) {
	for _, s := range []string{"10/m", "0", "1/s", "-1/h", "+3/m", "1e3/s", "", "/", "9999999/s"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := ParseRate(s)
		if err != nil {
			if !errors.Is(err, ErrInvalidRate) {
				t.Fatalf("unexpected error type: %v", err)
			}
			if r != (Rate{}) {
				t.Fatalf("non-zero rate on error: %+v", r)
			}
			return
		}
		if r == (Rate{}) {
			return
		}
		if r.Count < 1 || r.Count > maxRateCount || !r.Enabled() {
			t.Fatalf("out of range rate %+v from %q", r, s)
		}
	})
}
