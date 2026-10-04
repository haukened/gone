package config

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestValidIPPort(t *testing.T) {
	cleanGoneEnvForTest(t)
	v := validator.New()
	if err := v.RegisterValidation("ip_port", validIPPort); err != nil {
		t.Fatalf("register validation: %v", err)
	}
	for _, tc := range ipPortValidationCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Struct(&struct {
				Addr string `validate:"ip_port"`
			}{Addr: tc.addr})
			if tc.valid && err != nil {
				t.Fatalf("expected valid, got error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

type ipPortValidationCase struct {
	name  string
	addr  string
	valid bool
}

// ipPortValidationCases returns IP:port validation fixtures.
func ipPortValidationCases() []ipPortValidationCase {
	return []ipPortValidationCase{
		{name: "empty", addr: "", valid: false},
		{name: "missing_port", addr: "127.0.0.1", valid: false},
		{name: "missing_port_after_colon", addr: "127.0.0.1:", valid: false},
		{name: "just_colon_port", addr: ":8080", valid: true},
		{name: "loopback_ipv4", addr: "127.0.0.1:8080", valid: true},
		{name: "any_ipv4_low_port", addr: "0.0.0.0:1", valid: true},
		{name: "ipv6_loopback", addr: "[::1]:8080", valid: true},
		{name: "ipv6_any", addr: "[::]:443", valid: true},
		{name: "unbracketed_ipv6", addr: "::1:8080", valid: false},
		{name: "hostname_not_ip", addr: "localhost:8080", valid: false},
		{name: "invalid_host_chars", addr: "not_an_ip!:80", valid: false},
		{name: "non_numeric_port", addr: "127.0.0.1:http", valid: false},
		{name: "port_zero", addr: "127.0.0.1:0", valid: false},
		{name: "port_max_valid", addr: "127.0.0.1:65535", valid: true},
		{name: "port_overflow", addr: "127.0.0.1:65536", valid: false},
		{name: "negative_port", addr: "127.0.0.1:-1", valid: false},
		{name: "multi_leading_zero_port", addr: "127.0.0.1:00080", valid: true},
		{name: "space_prefixed", addr: " :8080", valid: false},
		{name: "trailing_space", addr: "127.0.0.1:8080 ", valid: false},
		{name: "embedded_space", addr: "127.0. 0.1:8080", valid: false},
	}
}
