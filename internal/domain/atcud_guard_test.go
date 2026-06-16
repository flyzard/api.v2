package domain

import (
	"testing"
	"time"
)

func TestNewATCUD_RejectsInvalidATCode(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	s := mustVal(NewSeries("A2026", FT))
	if err := s.RegisterWithAT("AAAABBBB", now); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	// Valid registered series mints an ATCUD.
	if _, err := NewATCUD(s, 1); err != nil {
		t.Fatalf("valid series: unexpected error: %v", err)
	}
	// Rehydrated/tampered series with a too-short / lowercase code must be rejected.
	s.ATCode = "abc"
	if _, err := NewATCUD(s, 1); err == nil {
		t.Error("NewATCUD must reject an AT code that fails ValidateATCode (len>=8 uppercase)")
	}
}
