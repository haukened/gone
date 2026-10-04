package config

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/v2"
)

var errConfigTestLoader = errors.New("config test loader error")

func TestLoadDefaultError(t *testing.T) {
	cleanGoneEnvForTest(t)
	orig := defaultLoader
	t.Cleanup(func() { defaultLoader = orig })
	defaultLoader = func(k *koanf.Koanf) error {
		if k == nil {
			t.Fatal("koanf must not be nil")
		}
		return errConfigTestLoader
	}
	assertLoadErrorIs(t, errConfigTestLoader)
}

func TestLoadEnvError(t *testing.T) {
	cleanGoneEnvForTest(t)
	orig := envLoader
	t.Cleanup(func() { envLoader = orig })
	envLoader = func(k *koanf.Koanf) error {
		if k == nil {
			t.Fatal("koanf must not be nil")
		}
		return errConfigTestLoader
	}
	assertLoadErrorIs(t, errConfigTestLoader)
}

func TestRegisterValidationFails(t *testing.T) {
	cleanGoneEnvForTest(t)
	orig := registerValidators
	t.Cleanup(func() { registerValidators = orig })
	registerValidators = func(v *validator.Validate) error {
		if v == nil {
			t.Fatal("validator must not be nil")
		}
		return errConfigTestLoader
	}
	assertLoadErrorIs(t, errConfigTestLoader)
}

// assertLoadErrorIs verifies Load returns an error wrapping want.
func assertLoadErrorIs(t *testing.T, want error) {
	t.Helper()
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got: %v", want, err)
	}
}
