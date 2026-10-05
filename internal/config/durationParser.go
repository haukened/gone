package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/haukened/gone/v3/internal/domain"
)

// StringToTTLOptions is a DecodeHookFunc that converts a string to domain.TTLOption
func StringToTTLOptions() mapstructure.DecodeHookFunc {
	return func(f, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t != reflect.TypeOf(domain.TTLOption{}) {
			return data, nil
		}
		s := strings.TrimSpace(data.(string))
		if s == "" {
			return nil, fmt.Errorf("empty TTL option string")
		}
		opt, err := domain.NewTTLOption(s)
		if err != nil {
			return nil, err
		}
		return opt, nil
	}
}
