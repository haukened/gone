package config

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.Addr != ":8080" || d.DataDir != "./data" || d.DSN == "" || d.MaxBytes != 128<<10 || d.MinTTL != time.Minute || d.MaxTTL != 7*24*time.Hour {
		t.Fatalf("defaults not as expected: %+v", d)
	}
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"131072":  131072,
		"128KiB":  128 * 1024,
		"128kib":  128 * 1024,
		"1MiB":    1024 * 1024,
		"2G":      2 * 1024 * 1024 * 1024,
		" 64KiB ": 64 * 1024,
		"10M":     10 * 1024 * 1024,
		"3GiB":    3 * 1024 * 1024 * 1024,
	}
	for in, want := range cases {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d, nil", in, got, err, want)
		}
	}
}

func TestParseSizeErrors(t *testing.T) {
	bad := []string{"", "-1", "12XB", "KIB", "123KB"}
	for _, in := range bad {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestParseSizePlainNegative(t *testing.T) {
	if _, err := ParseSize("-42"); err == nil {
		t.Fatalf("expected error for negative value")
	}
}

func TestFromEnvOverlay(t *testing.T) {
	base := Defaults()
	env := map[string]string{
		"GONE_ADDR":      "127.0.0.1:9000",
		"GONE_DATA_DIR":  "/tmp/gone",
		"GONE_DSN":       "file:test.db?_journal_mode=WAL&_fk=1",
		"GONE_MAX_BYTES": "256KiB",
		"GONE_MIN_TTL":   "2m",
		"GONE_MAX_TTL":   "48h",
	}
	getter := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if err := FromEnv(&base, getter); err != nil {
		t.Fatalf("FromEnv error: %v", err)
	}
	if base.Addr != "127.0.0.1:9000" || base.DataDir != "/tmp/gone" || base.MaxBytes != 256*1024 || base.MinTTL != 2*time.Minute || base.MaxTTL != 48*time.Hour {
		t.Fatalf("env overlay failed: %+v", base)
	}
}

func TestFromEnvError(t *testing.T) {
	c := Defaults()
	env := map[string]string{"GONE_MAX_BYTES": "12XB"}
	getter := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if err := FromEnv(&c, getter); err == nil || !strings.Contains(err.Error(), "GONE_MAX_BYTES") {
		t.Fatalf("expected wrapped error for GONE_MAX_BYTES, got %v", err)
	}
}

func TestFromEnvClearsOnEmpty(t *testing.T) {
	c := Defaults()
	env := map[string]string{"GONE_MAX_BYTES": "", "GONE_MIN_TTL": "", "GONE_MAX_TTL": ""}
	if err := FromEnv(&c, func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.MaxBytes != 0 || c.MinTTL != 0 || c.MaxTTL != 0 {
		t.Fatalf("expected cleared fields, got %+v", c)
	}
}

func TestApplyFlagsOverlay(t *testing.T) {
	c := Defaults()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fv := BindFlags(fs)
	args := []string{"-addr", "0.0.0.0:9999", "-max-bytes", "1MiB", "-min-ttl", "30s", "-max-ttl", "2h"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if err := ApplyFlags(&c, fv); err != nil {
		t.Fatalf("apply flags: %v", err)
	}
	if c.Addr != "0.0.0.0:9999" || c.MaxBytes != 1*1024*1024 || c.MinTTL != 30*time.Second || c.MaxTTL != 2*time.Hour {
		t.Fatalf("flag overlay failed: %+v", c)
	}
}

func TestApplyFlagsError(t *testing.T) {
	c := Defaults()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fv := BindFlags(fs)
	if err := fs.Parse([]string{"-max-bytes", "12XB"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ApplyFlags(&c, fv); err == nil || !strings.Contains(err.Error(), "-max-bytes") {
		t.Fatalf("expected wrapped flag error, got %v", err)
	}
}

func TestApplyFlagsNilConfig(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fv := BindFlags(fs)
	if err := fs.Parse([]string{"-addr", ":1"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ApplyFlags(nil, fv); err == nil || !strings.Contains(err.Error(), "nil config") {
		t.Fatalf("expected nil config error, got %v", err)
	}
}

func TestApplyFlagsNilFlagVals(t *testing.T) {
	c := Defaults()
	if err := ApplyFlags(&c, nil); err != nil {
		t.Fatalf("expected nil flag vals no-op, got %v", err)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	c := Config{} // empty => several errors
	errs := Validate(c)
	if len(errs) < 4 { // expect at least addr, data dir, max bytes, min ttl
		t.Fatalf("expected multiple errors, got %d", len(errs))
	}
}

func TestValidateMaxLessThanMin(t *testing.T) {
	c := Defaults()
	c.MaxTTL = c.MinTTL - time.Second
	errs := Validate(c)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), ">= min ttl") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected max ttl >= min ttl error, got %v", errs)
	}
}

func TestPrecedenceOrder(t *testing.T) {
	c := Defaults()
	// Pretend env overrides some, flags override again.
	env := map[string]string{
		"GONE_ADDR":      ":9001",
		"GONE_MAX_BYTES": "256KiB",
	}
	if err := FromEnv(&c, func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err != nil {
		t.Fatalf("env: %v", err)
	}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fv := BindFlags(fs)
	if err := fs.Parse([]string{"-addr", ":9002"}); err != nil {
		t.Fatalf("flags: %v", err)
	}
	if err := ApplyFlags(&c, fv); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if c.Addr != ":9002" {
		t.Fatalf("flags should win over env; got %s", c.Addr)
	}
	if c.MaxBytes != 256*1024 {
		t.Fatalf("env override for max bytes lost; got %d", c.MaxBytes)
	}
}

func TestGetenvPresent(t *testing.T) {
	key := "GONE_TEST_UNIQUE_KEY"
	_ = os.Unsetenv(key)
	if _, ok := GetenvPresent(key); ok {
		t.Fatalf("expected not present")
	}
	if err := os.Setenv(key, "value"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	if v, ok := GetenvPresent(key); !ok || v != "value" {
		t.Fatalf("unexpected getenv: %v %v", v, ok)
	}
	_ = os.Unsetenv(key)
}

func TestValidateSuccess(t *testing.T) {
	c := Defaults()
	errs := Validate(c)
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
}

// Ensure we didn't accidentally change exported struct fields.
func TestConfigStructFields(t *testing.T) {
	want := []string{"Addr", "DataDir", "DSN", "MaxBytes", "MinTTL", "MaxTTL"}
	var got []string
	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Config fields changed; got %v want %v", got, want)
	}
}
