package domain

import (
	"strings"
	"testing"
)

func TestDocumentLineValidate_RejectsOutOfRangePercent(t *testing.T) {
	l := gdTestLine(t, 1, 10.00, nil)
	l.Tax = mustVAT(t, TaxNormal)
	l.Discount = PercentDiscount{Rate: PercentScale + 1} // 100.01% — bypasses constructor

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Validate panicked instead of returning an error: %v", r)
		}
	}()
	err := l.Validate()
	if err == nil || !strings.Contains(err.Error(), "discount") {
		t.Fatalf("want discount validation error, got %v", err)
	}
}

func TestUnmarshalDiscount_PercentOutOfRangeRejected(t *testing.T) {
	_, err := unmarshalDiscount([]byte(`{"type":"percent","percent":20000}`))
	if err == nil {
		t.Fatal("decoded a 200% discount")
	}
}

func TestDiscountValidate(t *testing.T) {
	if err := (PercentDiscount{Rate: -1}).Validate(); err == nil {
		t.Error("negative percent accepted")
	}
	if err := (PercentDiscount{Rate: PercentScale}).Validate(); err != nil {
		t.Errorf("100%% rejected: %v", err)
	}
	if err := (AmountDiscount{Amount: -1}).Validate(); err == nil {
		t.Error("negative amount accepted")
	}
}
