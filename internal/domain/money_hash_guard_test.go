package domain

import (
	"encoding/json"
	"testing"
)

func TestMoneyUnmarshalJSON_RejectsOverflow(t *testing.T) {
	var m Money
	if err := json.Unmarshal([]byte("9223372036854775807"), &m); err == nil {
		t.Fatalf("overflowing cents accepted, got %d", m)
	}
	if err := json.Unmarshal([]byte("4950"), &m); err != nil || m != Money(4950*centScale) {
		t.Fatalf("valid cents rejected: %v / %d", err, m)
	}
}

func TestHashValidate_RejectsImplausibleSignatures(t *testing.T) {
	if err := Hash("x").Validate(); err == nil {
		t.Error("1-char hash accepted — FourChars would emit a 1-char QR field Q")
	}
	if err := Hash("not base64 !!! definitely not base64 at all here").Validate(); err == nil {
		t.Error("non-base64 hash accepted")
	}
	// A real RSA-1024 + base64 signature (172 chars) must pass.
	a, _, err := m16StubSigner{}.Sign("probe")
	if err != nil {
		t.Fatal(err)
	}
	if err := Hash(a).Validate(); err != nil {
		t.Errorf("stub RSA-shaped signature rejected: %v", err)
	}
}
