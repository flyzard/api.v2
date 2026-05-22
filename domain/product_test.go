package domain

import (
	"strings"
	"testing"
)

func TestUnitOfMeasureAcceptsAnyShortString(t *testing.T) {
	// Custom unit (not in the suggested constants) — must still validate.
	if err := UnitOfMeasure("CAIXA").Validate(); err != nil {
		t.Errorf("CAIXA should be accepted: %v", err)
	}
	if err := UnitOfMeasure("XYZ").Validate(); err != nil {
		t.Errorf("XYZ should be accepted: %v", err)
	}
}

func TestUnitOfMeasureAcceptsConstantValues(t *testing.T) {
	for _, u := range []UnitOfMeasure{UnitPiece, UnitKg, UnitHour, UnitService} {
		if err := u.Validate(); err != nil {
			t.Errorf("%s should be accepted: %v", u, err)
		}
	}
}

func TestUnitOfMeasureRejectsEmptyAndTooLong(t *testing.T) {
	if err := UnitOfMeasure("").Validate(); err == nil {
		t.Error("empty unit: expected error")
	}
	twentyOne := UnitOfMeasure(strings.Repeat("a", 21))
	if err := twentyOne.Validate(); err == nil {
		t.Error("21-char unit: expected error")
	}
	twenty := UnitOfMeasure(strings.Repeat("a", 20))
	if err := twenty.Validate(); err != nil {
		t.Errorf("20-char unit should be valid: %v", err)
	}
}

func TestNewProductAcceptsArbitraryUnit(t *testing.T) {
	// Previously only the closed enum was allowed; now any short string passes.
	_, err := NewProduct(Product{
		ProductCode:        "CODE1",
		ProductType:        ProductTypeGoods,
		ProductDescription: "Some product",
		ProductNumberCode:  "001",
		Unit:               "CAIXA",
	})
	if err != nil {
		t.Fatalf("custom unit on NewProduct: %v", err)
	}
}

func TestProductTypeIsValid(t *testing.T) {
	for _, pt := range []ProductType{
		ProductTypeGoods, ProductTypeService,
		ProductTypeOther, ProductTypeExcise, ProductTypeParafiscal,
	} {
		if !pt.IsValid() {
			t.Errorf("%s should be valid", pt)
		}
	}
	for _, pt := range []ProductType{"", "X", "p"} {
		if pt.IsValid() {
			t.Errorf("%q should not be valid", pt)
		}
	}
}

func TestNewProductRejectsOverlongUnit(t *testing.T) {
	long := UnitOfMeasure(strings.Repeat("a", 21))
	_, err := NewProduct(Product{
		ProductCode:        "CODE1",
		ProductType:        ProductTypeGoods,
		ProductDescription: "Some product",
		ProductNumberCode:  "001",
		Unit:               long,
	})
	if err == nil {
		t.Fatal("expected error for 21-char unit")
	}
}
