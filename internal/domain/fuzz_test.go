package domain

import (
	"testing"
)

// Seed corpora run under plain `go test`; use `go test -fuzz=FuzzName
// ./internal/domain` for an actual fuzzing session.

func FuzzParseDocNumber(f *testing.F) {
	f.Add("FT A2026/42")
	f.Add("GT G1/1")
	f.Add("RC R 2/3")
	f.Fuzz(func(t *testing.T, s string) {
		n, err := ParseDocNumber(s)
		if err != nil {
			return
		}
		back, err := ParseDocNumber(n.Format())
		if err != nil {
			t.Fatalf("Format output %q (from %q) does not re-parse: %v", n.Format(), s, err)
		}
		if back != n {
			t.Fatalf("round-trip drift: %+v vs %+v", n, back)
		}
	})
}

func FuzzRecoveredRefControl(f *testing.F) {
	f.Add(uint8('M'), "M2025", 7, "FT")
	f.Add(uint8('D'), "B2025", 9, "FT")
	f.Fuzz(func(t *testing.T, kind uint8, series string, num int, otype string) {
		ref := RecoveredRef{
			Kind:           RecoveryKind(kind),
			OriginalSeries: series,
			OriginalNumber: num,
			OriginalType:   DocumentType(otype),
		}
		if ref.Validate() != nil {
			return // invalid refs are rejected upstream; not this property's concern
		}
		hc := HashControl(ref.controlFor("1", FT))
		if err := hc.Validate(); err != nil {
			t.Fatalf("valid RecoveredRef produced invalid HashControl %q: %v", hc, err)
		}
	})
}

func FuzzProrateCents(f *testing.F) {
	f.Add(int64(300), uint16(5016), uint16(1000), uint16(0))
	f.Fuzz(func(t *testing.T, cents int64, w1, w2, w3 uint16) {
		weights := []Money{
			Money(int64(w1) * centScale),
			Money(int64(w2) * centScale),
			Money(int64(w3) * centScale),
		}
		var sum int64
		for _, w := range weights {
			sum += int64(w)
		}
		// Respect prorateCents' documented contract: non-negative weights with
		// a positive sum, 0 <= cents*centScale <= Σweights.
		if sum == 0 || cents < 0 || cents*centScale > sum {
			return
		}
		shares := prorateCents(cents, weights)
		var got int64
		for _, s := range shares {
			if s < 0 {
				t.Fatalf("negative share %d", s)
			}
			if int64(s)%centScale != 0 {
				t.Fatalf("share %d is not a whole cent", s)
			}
			got += int64(s)
		}
		if got != cents*centScale {
			t.Fatalf("Σshares = %d, want %d", got, cents*centScale)
		}
	})
}
