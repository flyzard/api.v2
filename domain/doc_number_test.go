package domain

import "testing"

func TestDocNumberFormat(t *testing.T) {
	d, err := NewDocNumber(FT, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Format(); got != "FT A/1" {
		t.Errorf("Format: got %q want FT A/1", got)
	}
}

func TestDocNumberRejects(t *testing.T) {
	cases := []struct {
		name   string
		typ    DocumentType
		series string
		seq    int
	}{
		{"invalid type", "XX", "A", 1},
		{"empty series", FT, "", 1},
		{"zero seq", FT, "A", 0},
		{"negative seq", FT, "A", -5},
		{"series w/ slash", FT, "A/B", 1},
		{"series w/ space", FT, "A B", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDocNumber(tc.typ, tc.series, tc.seq); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestDocNumberRegexFromXSD(t *testing.T) {
	// Validate the exact XSD pattern accepts canonical values.
	for _, s := range []string{"FT A/1", "NC SERIES.1/12345", "RG R-2024/7"} {
		if !docNumberPattern.MatchString(s) {
			t.Errorf("XSD pattern should match: %q", s)
		}
	}
	for _, s := range []string{"FT A/", "FT /1", " A/1", "FTA/1"} {
		if docNumberPattern.MatchString(s) {
			t.Errorf("XSD pattern should reject: %q", s)
		}
	}
}
