package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validVATLine(t *testing.T) DocumentLine {
	t.Helper()
	tax, err := NewVATLineTax(PT, TaxNormal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	product := Product{ProductCode: "PROD-1", ProductDescription: "Default test product"}
	return DocumentLine{
		LineNumber:   1,
		Product:      product,
		Description:  product.ProductDescription,
		Quantity:     mustQuantity(t, 1),
		UnitPrice:    mustMoney(t, 100),
		TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Tax:          tax,
	}
}

func TestDocumentLineRequiresTaxPointDate(t *testing.T) {
	l := validVATLine(t)
	l.TaxPointDate = time.Time{}
	if err := l.Validate(); err == nil {
		t.Fatal("missing TaxPointDate: expected error")
	}
}

func TestDocumentLineRejectsTaxBaseWithUnitPrice(t *testing.T) {
	l := validVATLine(t)
	tb := mustMoney(t, 50)
	l.TaxBase = &tb
	// l.UnitPrice is 100 — both non-zero is forbidden.
	if err := l.Validate(); err == nil {
		t.Fatal("TaxBase + non-zero UnitPrice: expected error")
	}
}

func TestDocumentLineAcceptsTaxBaseWhenUnitPriceZero(t *testing.T) {
	l := validVATLine(t)
	tb := mustMoney(t, 50)
	l.TaxBase = &tb
	l.UnitPrice = 0
	if err := l.Validate(); err != nil {
		t.Fatalf("TaxBase without UnitPrice: %v", err)
	}
}

func TestDocumentLineOrderReferenceLength(t *testing.T) {
	l := validVATLine(t)
	l.OrderReferences = []OrderReference{{OriginatingON: strings.Repeat("a", 61)}}
	if err := l.Validate(); err == nil {
		t.Fatal("originating_on > 60 chars: expected error")
	}
}

func TestDocumentLineReferenceLengths(t *testing.T) {
	l := validVATLine(t)
	l.References = []DocReference{{Reference: strings.Repeat("a", 61)}}
	if err := l.Validate(); err == nil {
		t.Fatal("reference > 60 chars: expected error")
	}
	l.References = []DocReference{{Reason: strings.Repeat("a", 51)}}
	if err := l.Validate(); err == nil {
		t.Fatal("reason > 50 chars: expected error")
	}
}

func TestDocumentLineSerialNumbers(t *testing.T) {
	l := validVATLine(t)
	l.SerialNumbers = []string{"SN-1", "SN-2"}
	if err := l.Validate(); err != nil {
		t.Fatalf("good serials: %v", err)
	}
	l.SerialNumbers = []string{""}
	if err := l.Validate(); err == nil {
		t.Fatal("empty serial: expected error")
	}
	l.SerialNumbers = []string{strings.Repeat("a", 101)}
	if err := l.Validate(); err == nil {
		t.Fatal("serial > 100 chars: expected error")
	}
}

func TestLine_RejectsTooLong(t *testing.T) {
	l := validVATLine(t)
	long := strings.Repeat("a", 201)
	l.Product.ProductDescription = long
	l.Description = long
	if err := l.Validate(); err == nil {
		t.Fatal("201-char description: expected error")
	}
}

func TestLine_RejectsDescriptionDrift(t *testing.T) {
	l := validVATLine(t)
	l.Description = "drifted from product"
	// l.Product.ProductDescription stays "Default test product"
	if err := l.Validate(); err == nil {
		t.Fatal("expected drift error")
	}
}

func TestDocumentLineUnmarshalCallsValidate(t *testing.T) {
	// Missing tax_point_date — UnmarshalJSON should reject.
	raw := `{
		"line_number": 1,
		"quantity": 1,
		"unit_price": 100,
		"tax": {"type": "IVA", "vat": {"rate": {"region": "PT", "category": "NOR"}}}
	}`
	var l DocumentLine
	if err := json.Unmarshal([]byte(raw), &l); err == nil {
		t.Fatal("expected error for missing tax_point_date")
	}
}
