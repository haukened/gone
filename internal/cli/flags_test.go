package cli

import (
	"errors"
	"flag"
	"slices"
	"testing"
)

// TestParseArgs checks interleaved positionals, "--", help and bad flags.
//
// Parameters:
//   - t: the test handle.
func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr int
	}{
		{"empty", nil, nil, exitOK},
		{"interleaved", []string{"a", "--v", "b"}, []string{"a", "b"}, exitOK},
		{"double dash", []string{"a", "--", "--v", "-x"}, []string{"a", "--v", "-x"}, exitOK},
		{"leading double dash", []string{"--", "b"}, []string{"b"}, exitOK},
		{"bad flag", []string{"--nope"}, nil, exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := newFlagSet("x")
			fs.Bool("v", false, "")
			got, err := parseArgs(fs, tt.args)
			if tt.wantErr != exitOK {
				if err == nil || classify(err).code != tt.wantErr {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	fs := newFlagSet("x")
	if _, err := parseArgs(fs, []string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help err = %v", err)
	}
	fs.Usage()
}

// TestOnePositional checks the single-positional helper.
//
// Parameters:
//   - t: the test handle.
func TestOnePositional(t *testing.T) {
	if v, err := onePositional(newFlagSet("x"), []string{"a"}, "link"); err != nil || v != "a" {
		t.Fatalf("got %q, %v", v, err)
	}
	for _, args := range [][]string{nil, {"a", "b"}, {"--bad"}} {
		if _, err := onePositional(newFlagSet("x"), args, "link"); err == nil {
			t.Errorf("onePositional(%q) accepted", args)
		}
	}
}

// TestValidateCommonAndStringList checks timeout validation and the
// repeatable flag type.
//
// Parameters:
//   - t: the test handle.
func TestValidateCommonAndStringList(t *testing.T) {
	if err := validateCommon(common{timeout: 0}); classify(err).code != exitUsage {
		t.Fatalf("zero timeout err = %v", err)
	}
	if err := validateCommon(common{timeout: 1}); err != nil {
		t.Fatal(err)
	}
	var s stringList
	_ = s.Set("a")
	_ = s.Set("b")
	if s.String() != "a,b" {
		t.Fatalf("String = %q", s.String())
	}
}
