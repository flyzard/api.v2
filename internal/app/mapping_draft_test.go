package app

import (
	"testing"
)

func TestSalesDraftAndViewRoundTrip(t *testing.T) {
	in := IssueInvoiceInput{
		DocType: DocFT, SeriesID: "FT2026", Date: "2026-05-20",
		IssuedBy: UserInput{"issuer@demo.pt", "Maria"},
		Customer: realNIF(t),
		Lines: []LineInput{{
			ProductCode: "P003", ProductType: ProductGoods, ProductDescription: "Vinho",
			ProductNumberCode: "P003", Unit: UnitPiece, Quantity: 6, UnitPriceCents: 890,
			TaxPointDate: "2026-05-20", Tax: &LineTaxInput{Kind: "VAT", Region: RegionPT, Category: RateIntermediate},
		}},
	}
	d, err := salesDraftFrom(in)
	if err != nil {
		t.Fatalf("salesDraftFrom: %v", err)
	}
	if d.DocumentType != "FT" || len(d.Lines) != 1 {
		t.Fatalf("draft shape wrong: %+v", d)
	}
	if d.Lines[0].UnitPrice.Cents() != 890 {
		t.Fatalf("unit price = %d cents, want 890", d.Lines[0].UnitPrice.Cents())
	}
}

func TestWorkDraftBuilder(t *testing.T) {
	in := IssueWorkInput{
		DocType:  DocOR,
		SeriesID: "OR2026",
		Date:     "2026-05-20",
		IssuedBy: UserInput{"issuer@demo.pt", "Maria"},
		Customer: realNIF(t),
		Lines: []LineInput{
			{
				ProductCode: "SVC001", ProductType: ProductService, ProductDescription: "Orçamento de obras",
				ProductNumberCode: "SVC001", Unit: UnitHour, Quantity: 2, UnitPriceCents: 5000,
				TaxPointDate: "2026-05-20", Tax: &LineTaxInput{Kind: "VAT", Region: RegionPT, Category: RateNormal},
			},
		},
	}
	d, err := workDraftFrom(in)
	if err != nil {
		t.Fatalf("workDraftFrom: %v", err)
	}
	if d.DocumentType != "OR" {
		t.Fatalf("expected doc type OR, got %s", d.DocumentType)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
}

func TestStockDraftBuilder(t *testing.T) {
	in := IssueStockInput{
		DocType:  DocGT,
		SeriesID: "GT2026",
		Date:     "2026-05-21",
		IssuedBy: UserInput{"issuer@demo.pt", "Maria"},
		Customer: realNIF(t),
		Lines: []LineInput{
			{
				ProductCode: "P001", ProductType: ProductGoods, ProductDescription: "Caixas de vinho",
				ProductNumberCode: "P001", Unit: UnitPiece, Quantity: 10, UnitPriceCents: 0,
				TaxPointDate: "2026-05-21",
			},
		},
		MovementStartTime: "2026-05-21T14:00:00Z",
		ShipFrom:          &AddressInput{Detail: "Rua da Adega, 1", City: "Lisboa", PostalCode: "1000-001"},
		ShipTo:            &AddressInput{Detail: "Av. das Flores, 50", City: "Porto", PostalCode: "4000-100"},
	}
	d, err := stockDraftFrom(in)
	if err != nil {
		t.Fatalf("stockDraftFrom: %v", err)
	}
	if d.DocumentType != "GT" {
		t.Fatalf("expected doc type GT, got %s", d.DocumentType)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
	if d.ShipFrom == nil {
		t.Fatal("ShipFrom must not be nil")
	}
}

func TestPaymentDraftBuilder(t *testing.T) {
	in := IssuePaymentInput{
		Type:            DocRC,
		SeriesID:        "RC2026",
		TransactionDate: "2026-05-20",
		Customer:        realNIF(t),
		SourceID:        "FT FT2026/1",
		Methods: []PaymentMethodInput{
			{Mechanism: MechMultibanco, AmountCents: 1065, Date: "2026-05-20"},
		},
		Lines: []PaymentLineInput{
			{
				LineNumber:  1,
				CreditCents: 1065,
				Tax:         &LineTaxInput{Kind: "VAT", Region: RegionPT, Category: RateIntermediate},
				SourceDocuments: []SourceDocInput{
					{OriginatingON: "FT FT2026/1", InvoiceDate: "2026-05-20", Description: "Vinho"},
				},
			},
		},
		Totals: TotalsInput{NetCents: 950, TaxCents: 115, GrossCents: 1065},
	}
	d, err := paymentDraftFrom(in)
	if err != nil {
		t.Fatalf("paymentDraftFrom: %v", err)
	}
	if d.Type != "RC" {
		t.Fatalf("expected type RC, got %s", d.Type)
	}
	if len(d.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(d.Methods))
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
}
