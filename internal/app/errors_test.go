package app

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorCarriesCode(t *testing.T) {
	e := newErrorCode(KindInvalid, "invalid_tax_id", errors.New("bad nif"))
	if e.Code != "invalid_tax_id" {
		t.Fatalf("Code = %q, want invalid_tax_id", e.Code)
	}
	if KindOf(e) != KindInvalid {
		t.Fatalf("KindOf = %v, want KindInvalid", KindOf(e))
	}
}

func TestVersionConflictReachableThroughAppError(t *testing.T) {
	wrapped := newError(KindConflict, fmt.Errorf("exhausted: %w", ErrVersionConflict))
	if !errors.Is(wrapped, ErrVersionConflict) {
		t.Fatal("a stateful consumer cannot detect a version conflict to refetch")
	}
}
