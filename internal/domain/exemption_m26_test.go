package domain

import "testing"

func TestExemption_M26(t *testing.T) {
	if !M26.Valid() {
		t.Fatal("M26 must be a valid exemption code (Lei 17/2023 cabaz alimentar)")
	}
	if M26.Description() == "" {
		t.Error("M26 must have a Description")
	}
	if M26.IsReverseCharge() {
		t.Error("M26 is an exemption with deduction right, not reverse charge")
	}
}
