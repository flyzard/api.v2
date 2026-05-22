package domain

import (
	"strings"
	"testing"
)

func TestHashValidate(t *testing.T) {
	if err := Hash("").Validate(); err == nil {
		t.Error("empty hash: expected error")
	}
	if err := Hash("abc").Validate(); err != nil {
		t.Errorf("short hash should be valid: %v", err)
	}
	if err := Hash(strings.Repeat("x", 173)).Validate(); err == nil {
		t.Error("over 172 chars: expected error")
	}
	if err := Hash(strings.Repeat("x", 172)).Validate(); err != nil {
		t.Errorf("exactly 172 chars should be valid: %v", err)
	}
}

func TestHashControlValidate(t *testing.T) {
	good := []string{
		"1",
		"0",
		"1.0",
		"2.1",
		"1-AAM ABC/1",      // manual emission marker
		"1-AAD FT ABC/1",   // recovery emission marker
	}
	for _, s := range good {
		if err := HashControl(s).Validate(); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	bad := []string{
		"",
		"abc",
		"1-aaM ABC/1",  // lowercase letters
		"1-AAX ABC/1",  // wrong marker (must be M or D)
		"1.a",
		"1-AA M ABC/1", // space between letters and marker — XSD forbids
	}
	for _, s := range bad {
		if err := HashControl(s).Validate(); err == nil {
			t.Errorf("%q should be invalid", s)
		}
	}
}
