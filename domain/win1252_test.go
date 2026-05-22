package domain

import (
	"strings"
	"testing"
)

func TestEnforceWindows1252(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", false},
		{"ascii", "Lisboa", false},
		{"latin1 letters", "São Paulo", false},  // ã is U+00E3, in Win-1252
		{"win1252 specials", "€ƒœŠ", false},     // 0x80, 0x83, 0x9C, 0x8A
		{"cjk", "北京", true},                     // not in Win-1252
		{"emoji", "Lisboa 🇵🇹", true},            // not in Win-1252
		{"undefined slot U+0081", "bad", true}, // Win-1252 0x81 is undefined
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceWindows1252(tc.in, "field")
			if (err != nil) != tc.wantErr {
				t.Errorf("enforceWindows1252(%q): err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestAddress_RejectsNonWin1252(t *testing.T) {
	_, err := NewAddress("Rua A", "北京", "1000-001", "PT")
	if err == nil {
		t.Fatal("expected error for CJK city")
	}
	if !strings.Contains(err.Error(), "city") {
		t.Errorf("error should name the field, got: %v", err)
	}
}

func TestAddress_PTPostalCodeFormat(t *testing.T) {
	if _, err := NewAddress("Rua A", "Lisboa", "1000001", "PT"); err == nil {
		t.Fatal("PT postal code without dash should be rejected")
	}
	if _, err := NewAddress("Rua A", "Lisboa", "1000-001", "PT"); err != nil {
		t.Fatalf("valid PT postal code rejected: %v", err)
	}
	// Non-PT countries are not subject to the dash format.
	if _, err := NewAddress("Calle A", "Madrid", "28013", "ES"); err != nil {
		t.Fatalf("ES postal code 28013 should pass: %v", err)
	}
}

func TestAddress_AcceptsLatin1Accents(t *testing.T) {
	_, err := NewAddress("Avenida da Liberdade", "São Paulo", "1000-001", "PT")
	if err != nil {
		t.Fatalf("Latin-1 accented chars must be accepted: %v", err)
	}
}
