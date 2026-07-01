package app

import "testing"

func TestEnumTablesRoundTripToDomain(t *testing.T) {
	// every doc-type const maps to its domain value; unknowns are rejected.
	for s, want := range docTypes {
		got, err := mapDocType(s)
		if err != nil || got != want {
			t.Fatalf("mapDocType(%q) = %v, %v", s, got, err)
		}
		if string(got) != s { // the const value IS the domain string
			t.Fatalf("doc type %q != %q", got, s)
		}
	}
	if _, err := mapDocType("ZZ"); err == nil || KindOf(err) != KindInvalid {
		t.Fatal("unknown doc type must map to KindInvalid")
	}
}

func TestTaxRateTableMatchesDomain(t *testing.T) {
	for s := range categories {
		if _, err := mapCategory(s); err != nil {
			t.Fatalf("mapCategory(%q): %v", s, err)
		}
	}
	if _, err := mapCategory("HUGE"); err == nil {
		t.Fatal("unknown rate must error")
	}
}

func TestRegionTableRoundTrip(t *testing.T) {
	for s := range regions {
		if _, err := mapRegion(s); err != nil {
			t.Fatalf("mapRegion(%q): %v", s, err)
		}
	}
	if _, err := mapRegion("XX"); err == nil || KindOf(err) != KindInvalid {
		t.Fatal("unknown region must return KindInvalid")
	}
}

func TestExemptionTableRoundTrip(t *testing.T) {
	for s := range exemptions {
		if _, err := mapExemption(s); err != nil {
			t.Fatalf("mapExemption(%q): %v", s, err)
		}
	}
	if _, err := mapExemption("M00"); err == nil || KindOf(err) != KindInvalid {
		t.Fatal("unknown exemption must return KindInvalid")
	}
}

func TestMechanismTableRoundTrip(t *testing.T) {
	for s := range mechanisms {
		if _, err := mapMechanism(s); err != nil {
			t.Fatalf("mapMechanism(%q): %v", s, err)
		}
	}
	if _, err := mapMechanism("WIRE"); err == nil || KindOf(err) != KindInvalid {
		t.Fatal("unknown mechanism must return KindInvalid")
	}
}

func TestUnitTableRoundTrip(t *testing.T) {
	for s := range units {
		if _, err := mapUnit(s); err != nil {
			t.Fatalf("mapUnit(%q): %v", s, err)
		}
	}
	if _, err := mapUnit("KG"); err == nil || KindOf(err) != KindInvalid {
		t.Fatal("unknown unit must return KindInvalid")
	}
}

func TestProductTypeTableRoundTrip(t *testing.T) {
	for s := range productTypes {
		if _, err := mapProductType(s); err != nil {
			t.Fatalf("mapProductType(%q): %v", s, err)
		}
	}
	if _, err := mapProductType("DIGITAL"); err == nil || KindOf(err) != KindInvalid {
		t.Fatal("unknown product type must return KindInvalid")
	}
}
