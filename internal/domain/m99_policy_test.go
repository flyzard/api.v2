package domain

import "testing"

// The engine must NOT silently default an exempt line to M99 (Despacho 8632/2014
// §3.2.6: the issuer must state the reason). M99 is accepted only when chosen
// explicitly; an empty exemption on an exempt line is rejected.
func TestGetTaxRate_M99NoSilentDefault(t *testing.T) {
	// Explicit M99 is accepted.
	rate, err := GetTaxRate(PT, TaxExempt, M99)
	if err != nil {
		t.Fatalf("explicit M99 must be accepted: %v", err)
	}
	if rate.Exemption != M99 {
		t.Errorf("rate.Exemption = %q, want M99", rate.Exemption)
	}
	// Empty exemption on an exempt line is rejected (no silent M99 default).
	if _, err := GetTaxRate(PT, TaxExempt, ""); err == nil {
		t.Error("empty exemption on an exempt line must be rejected, not defaulted to M99")
	}
}
