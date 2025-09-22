package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/haukened/gone/internal/domain"
	"github.com/mitchellh/mapstructure"
)

// TestStringToTTLOptions covers the DecodeHook behavior for various inputs.
func TestStringToTTLOptions(t *testing.T) {
	tests := []struct {
		name       string
		fromType   reflect.Type
		toType     reflect.Type
		input      interface{}
		expectVal  interface{}
		expectErr  bool
		verifyFunc func(got interface{}) error
	}{
		{
			name:      "valid duration minutes",
			fromType:  reflect.TypeOf(""),
			toType:    reflect.TypeOf(domain.TTLOption{}),
			input:     "5m",
			expectErr: false,
			expectVal: domain.TTLOption{Duration: 5 * time.Minute, Label: "5m"},
		},
		{
			name:      "valid duration hours",
			fromType:  reflect.TypeOf(""),
			toType:    reflect.TypeOf(domain.TTLOption{}),
			input:     "1h30m",
			expectErr: false,
			expectVal: domain.TTLOption{Duration: 90 * time.Minute, Label: "1h30m"},
		},
		{
			name:      "empty string",
			fromType:  reflect.TypeOf(""),
			toType:    reflect.TypeOf(domain.TTLOption{}),
			input:     "",
			expectErr: true,
		},
		{
			name:      "whitespace string",
			fromType:  reflect.TypeOf(""),
			toType:    reflect.TypeOf(domain.TTLOption{}),
			input:     "   ",
			expectErr: true,
		},
		{
			name:      "unsupported unit days",
			fromType:  reflect.TypeOf(""),
			toType:    reflect.TypeOf(domain.TTLOption{}),
			input:     "2d",
			expectErr: true,
		},
		{
			name:      "not this type",
			fromType:  reflect.TypeOf(""),
			toType:    reflect.TypeOf(0),
			input:     "something_else",
			expectErr: false,
			expectVal: "something_else",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fromVal := reflect.ValueOf(tt.input)
			toVal := reflect.New(tt.toType).Elem()
			got, err := mapstructure.DecodeHookExec(StringToTTLOptions(), fromVal, toVal)

			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%v)", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.verifyFunc != nil {
				if vErr := tt.verifyFunc(got); vErr != nil {
					t.Errorf("verification failed: %v", vErr)
				}
				return
			}

			if !reflect.DeepEqual(got, tt.expectVal) {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expectVal, tt.expectVal, got, got)
			}
		})
	}
}
