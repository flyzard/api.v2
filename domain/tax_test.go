package domain

import "testing"

func TestGetTaxRate_Madeira(t *testing.T) {
	rate, err := GetTaxRate(PTMA, TaxNormal, "")
	if err != nil {
		t.Fatalf("PTMA normal rate lookup: %v", err)
	}
	if rate.Region != PTMA {
		t.Errorf("region: got %s, want %s", rate.Region, PTMA)
	}
	if rate.Value != 2200 {
		t.Errorf("value: got %d, want 2200", rate.Value)
	}
}

func TestPTMAJurisdictionAccepted(t *testing.T) {
	if !TaxJurisdiction("PT-MA").IsValid() {
		t.Fatal(`TaxJurisdiction("PT-MA") should be valid`)
	}
}

func TestTaxOther_DirectConstructionPasses(t *testing.T) {
	r := TaxRate{Region: PT, Category: TaxOther, Value: 1700}
	if err := r.Validate(); err != nil {
		t.Fatalf("OUT with caller-declared rate must pass: %v", err)
	}
}

func TestGetTaxRate_RejectsTaxOther(t *testing.T) {
	if _, err := GetTaxRate(PT, TaxOther, ""); err == nil {
		t.Fatal("GetTaxRate should refuse OUT")
	}
}

func TestTaxOther_RejectsUnknownRegion(t *testing.T) {
	r := TaxRate{Region: "XX", Category: TaxOther, Value: 1000}
	if err := r.Validate(); err == nil {
		t.Fatal("unknown region must be rejected even for OUT")
	}
}
