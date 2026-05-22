package domain

import (
	"encoding/json"
	"testing"
)

func TestVATLineTaxNormalRate(t *testing.T) {
	lt, err := NewVATLineTax(PT, TaxNormal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	v, ok := lt.(VATTax)
	if !ok {
		t.Fatalf("expected VATTax, got %T", lt)
	}
	if v.Rate.Category != TaxNormal {
		t.Errorf("category: got %s", v.Rate.Category)
	}
	base, _ := NewMoney(100)
	got := lt.Apply(base)
	want, _ := NewMoney(23)
	if got != want {
		t.Errorf("Apply: got %v want %v", got, want)
	}
}

func TestVATLineTaxExemptRequiresReasonText(t *testing.T) {
	if _, err := NewVATLineTax(PT, TaxExempt, M07, ""); err == nil {
		t.Fatal("expected error for exempt with no reason text")
	}
	if _, err := NewVATLineTax(PT, TaxExempt, M07, "short"); err == nil {
		t.Fatal("expected error for too-short reason")
	}
	if _, err := NewVATLineTax(PT, TaxExempt, M07, "Isento artigo 9 do CIVA"); err != nil {
		t.Fatalf("valid exempt: %v", err)
	}
}

func TestStampLineTax(t *testing.T) {
	amt, _ := NewMoney(5)
	lt, err := NewStampLineTax("PT", "IS-G", amt)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lt.(StampTax); !ok {
		t.Fatalf("expected StampTax, got %T", lt)
	}
	// Stamp is a fixed amount regardless of base.
	base, _ := NewMoney(999)
	if got := lt.Apply(base); got != amt {
		t.Errorf("stamp Apply: got %v want %v", got, amt)
	}
}

func TestStampLineTaxRejectsBadJurisdiction(t *testing.T) {
	amt, _ := NewMoney(5)
	if _, err := NewStampLineTax("ZZ", "IS-G", amt); err == nil {
		t.Fatal("invalid jurisdiction: expected error")
	}
	if _, err := NewStampLineTax("Desconhecido", "IS-G", amt); err == nil {
		t.Fatal("Desconhecido is not a valid tax jurisdiction: expected error")
	}
}

func TestStampLineTaxAcceptsRegionalJurisdiction(t *testing.T) {
	amt, _ := NewMoney(1)
	for _, j := range []TaxJurisdiction{"PT-AC", "PT-MA", "PT", "ES"} {
		if _, err := NewStampLineTax(j, "IS-G", amt); err != nil {
			t.Errorf("jurisdiction %s: %v", j, err)
		}
	}
}

func TestNotSubjectLineTax(t *testing.T) {
	lt, err := NewNotSubjectLineTax("PT", M99, "Não sujeito por X")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lt.(NotSubjectTax); !ok {
		t.Fatalf("expected NotSubjectTax, got %T", lt)
	}
	base, _ := NewMoney(100)
	if lt.Apply(base) != 0 {
		t.Error("NS Apply should be 0")
	}
}

func TestLineTaxJSONRoundTrip(t *testing.T) {
	lt, err := NewVATLineTax(PT, TaxIntermediate, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(lt)
	if err != nil {
		t.Fatal(err)
	}
	back, err := unmarshalLineTax(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("round-trip Validate: %v", err)
	}
	if back.(VATTax).Rate.Value != lt.(VATTax).Rate.Value {
		t.Errorf("rate value lost in round-trip")
	}
}
