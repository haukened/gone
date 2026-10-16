package sqlite

import (
	"errors"
	"testing"

	"github.com/haukened/gone/v3/internal/app"
	"github.com/haukened/gone/v3/internal/store"
)

func TestDeadRowError(t *testing.T) {
	var err error = deadRowError{external: true}
	if err.Error() == "" {
		t.Fatalf("empty message")
	}
	if got := (deadRowError{}).notFound(); !errors.Is(got, app.ErrNotFound) || errors.Is(got, store.ErrDeadExternal) {
		t.Fatalf("inline notFound = %v", got)
	}
	if got := (deadRowError{external: true}).notFound(); !errors.Is(got, store.ErrDeadExternal) {
		t.Fatalf("external notFound = %v", got)
	}
}
