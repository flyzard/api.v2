package app

import (
	"errors"
	"fmt"
	"testing"
)

func TestKindOf(t *testing.T) {
	if got := KindOf(newError(KindInvalid, errors.New("x"))); got != KindInvalid {
		t.Fatalf("KindOf = %v, want KindInvalid", got)
	}
	if got := KindOf(errors.New("plain")); got != KindInternal {
		t.Fatalf("KindOf(plain) = %v, want KindInternal", got)
	}
}

func TestErrorUnwrap(t *testing.T) {
	err := newError(KindConflict, fmt.Errorf("wrap: %w", ErrSeriesNotIssuable))
	if !errors.Is(err, ErrSeriesNotIssuable) {
		t.Fatal("Error must unwrap to its sentinel")
	}
}
