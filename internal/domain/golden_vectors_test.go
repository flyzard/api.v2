package domain

import (
	"testing"
	"time"
)

// These tests pin regulatory-frozen artifacts byte-for-byte (Portaria
// 363/2010). If one fails after a refactor, the refactor changed certified
// output — that is a regulatory bug to fix, never a test to update.

func TestCanonicalHashInput_Golden(t *testing.T) {
	date := time.Date(2026, 5, 10, 0, 0, 0, 0, lisbonLocation)
	sys := time.Date(2026, 5, 10, 14, 30, 5, 0, lisbonLocation)
	got := canonicalHashInput(date, sys, "FT A2026/42", Money(246*scale), "PREVHASH==")
	want := "2026-05-10;2026-05-10T14:30:05;FT A2026/42;246.00;PREVHASH=="
	if got != want {
		t.Fatalf("canonical string changed:\n got: %q\nwant: %q", got, want)
	}
}

func TestHashFourChars_Golden(t *testing.T) {
	// 1-based positions 1, 11, 21, 31 — NOT the first four characters.
	h := Hash("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef")
	if got := h.FourChars(); got != "AKUe" {
		t.Fatalf("FourChars = %q, want %q", got, "AKUe")
	}
}

func TestRecoveredControlFor_Golden(t *testing.T) {
	manual := RecoveredRef{Kind: RecoveryManual, OriginalSeries: "M2025", OriginalNumber: 7}
	if got := manual.controlFor("1", FT); got != "1-FTM M2025/7" {
		t.Fatalf("manual control = %q, want %q", got, "1-FTM M2025/7")
	}
	if err := HashControl(manual.controlFor("1", FT)).Validate(); err != nil {
		t.Fatalf("manual control rejected by own pattern: %v", err)
	}

	backup := RecoveredRef{Kind: RecoveryBackup, OriginalSeries: "B2025", OriginalNumber: 9, OriginalType: FT}
	if got := backup.controlFor("1", FT); got != "1-FTD FT B2025/9" {
		t.Fatalf("backup control = %q, want %q", got, "1-FTD FT B2025/9")
	}
	if err := HashControl(backup.controlFor("1", FT)).Validate(); err != nil {
		t.Fatalf("backup control rejected by own pattern: %v", err)
	}
}
