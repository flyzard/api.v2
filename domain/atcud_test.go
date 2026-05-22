package domain

import (
	"testing"
	"time"
)

func TestATCUDUnregisteredSeriesErrors(t *testing.T) {
	s, err := NewSeries("A", FT)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewATCUD(s, 1); err == nil {
		t.Fatal("unregistered series should error (AUDIT 3.14)")
	}
}

func TestATCUDRegisteredSeries(t *testing.T) {
	s, err := NewSeries("A", FT)
	if err != nil {
		t.Fatal(err)
	}
	s.ATCode = "AAJ7H5K2"
	got, err := NewATCUD(s, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got != "AAJ7H5K2-42" {
		t.Errorf("got %q want AAJ7H5K2-42", got)
	}
}

func TestATCUDRejectsBadSeq(t *testing.T) {
	s, _ := NewSeries("A", FT)
	if _, err := NewATCUD(s, 0); err == nil {
		t.Error("seq 0: expected error")
	}
	if _, err := NewATCUD(s, -1); err == nil {
		t.Error("seq -1: expected error")
	}
}

func TestValidateATCode(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{"valid 8 char", "AAJ7H5K2", false},
		{"valid 12 char", "ABCD1234EFGH", false},
		{"too short", "AAJ7H5K", true},
		{"empty", "", true},
		{"lowercase", "aaj7h5k2", true},
		{"with hyphen", "AAJ7-K2X", true},
		{"with space", "AAJ7 K2X", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateATCode(tc.code)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateATCode(%q) err=%v wantErr=%v", tc.code, err, tc.wantErr)
			}
		})
	}
}

func TestRegisterWithAT_RejectsInvalidCode(t *testing.T) {
	s, _ := NewSeries("A", FT)
	if err := s.RegisterWithAT("short", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected error for too-short AT code")
	}
	if s.ATCode != "" {
		t.Fatal("series should not mutate on invalid AT code")
	}
}
