package domain

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- SalesInvoice ---

func TestIssueSalesInvoiceFT(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	inv, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if inv.DocumentType != FT {
		t.Errorf("doctype: %s", inv.DocumentType)
	}
	if inv.Status != StatusNormal {
		t.Errorf("status: %s", inv.Status)
	}
}

func TestIssueSalesInvoiceRejectsNonSalesType(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = GT // transport, not sales
	s := registeredSeries(t, "A", GT)
	d.Series = *s
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	_, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected error for non-sales doc type")
	}
}

func TestSalesInvoiceNCRequiresLineReferences(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = NC
	s := registeredSeries(t, "A", NC)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	// NC line without References should fail validation.
	_, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err == nil {
		t.Fatal("expected error for NC without References on lines")
	}
}

func TestSalesInvoiceNCWithReferencesPasses(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = NC
	d.Lines[0].References = []DocReference{{Reference: "FT A/1", Reason: "correction"}}
	s := registeredSeries(t, "A", NC)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	if _, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{}); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestDraftFR_RequiresPayment(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = FR
	s := registeredSeries(t, "FR1", FR)
	d.Series = *s
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	if _, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{}); err == nil {
		t.Fatal("FR without Payments should error")
	}
}

func TestDraftFR_AmountMatchesGross(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = FR
	s := registeredSeries(t, "FR1", FR)
	d.Series = *s
	draft := &DraftSalesInvoice{
		CommonDraftDocument: *d,
		SalesInvoiceFields: SalesInvoiceFields{
			Payments: []FRPayment{
				{Mechanism: PaymentMechanismCash, Amount: mustMoney(t, 100), Date: d.Date},
			},
		},
	}
	if _, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{}); err == nil {
		t.Fatal("FR payment sum (100) != gross (123) should error")
	}
}

func TestDraftSalesInvoice_ValidateIsIdempotent(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = FR
	s := registeredSeries(t, "FR1", FR)
	d.Series = *s
	draft := &DraftSalesInvoice{
		CommonDraftDocument: *d,
		SalesInvoiceFields: SalesInvoiceFields{
			Payments: []FRPayment{
				{Mechanism: PaymentMechanismCash, Amount: mustMoney(t, 123), Date: d.Date},
			},
		},
	}
	if err := draft.Validate(); err != nil {
		t.Fatalf("first Validate: %v", err)
	}
	if err := draft.Validate(); err != nil {
		t.Fatalf("second Validate: %v", err)
	}
	// Validate must be read-only: Totals stay at the zero value.
	if draft.Totals.GrossTotal != 0 || draft.Totals.NetTotal != 0 || draft.Totals.TaxTotal != 0 {
		t.Fatalf("Validate mutated Totals; got %+v", draft.Totals)
	}
}

func TestDraftFR_MultipleMethodsSumToGross(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = FR
	s := registeredSeries(t, "FR1", FR)
	d.Series = *s
	draft := &DraftSalesInvoice{
		CommonDraftDocument: *d,
		SalesInvoiceFields: SalesInvoiceFields{
			Payments: []FRPayment{
				{Mechanism: PaymentMechanismCash, Amount: mustMoney(t, 50), Date: d.Date},
				{Mechanism: PaymentMechanismCreditCard, Amount: mustMoney(t, 73), Date: d.Date},
			},
		},
	}
	if _, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{}); err != nil {
		t.Fatalf("FR with 50+73=123 matching gross 123 should pass: %v", err)
	}
}

// fakeReader is a test-only IssuedDocumentReader backed by a map.
type fakeReader struct{ docs map[string]IssuedDocument }

func (r fakeReader) FindByNumber(n DocNumber) (IssuedDocument, error) {
	d, ok := r.docs[n.Format()]
	if !ok {
		return IssuedDocument{}, fmt.Errorf("doc %q not found", n.Format())
	}
	return d, nil
}

func ndDraftWithRef(t *testing.T, productCode string, qty Quantity) *DraftSalesInvoice {
	t.Helper()
	d := validDraft(t)
	d.DocumentType = ND
	desc := "Test product " + productCode
	d.Lines[0].Product = Product{ProductCode: productCode, ProductDescription: desc}
	d.Lines[0].Description = desc
	d.Lines[0].Quantity = qty
	d.Lines[0].References = []DocReference{{Reference: "FT A/1", Reason: "value adjustment"}}
	s := registeredSeries(t, "ND1", ND)
	d.Series = *s
	return &DraftSalesInvoice{CommonDraftDocument: *d}
}

func originatingInvoice(t *testing.T, productCode string, qty Quantity) IssuedDocument {
	t.Helper()
	d := validDraft(t)
	desc := "Test product " + productCode
	d.Lines[0].Product = Product{ProductCode: productCode, ProductDescription: desc}
	d.Lines[0].Description = desc
	d.Lines[0].Quantity = qty
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

func TestDraftND_RejectsNewProduct(t *testing.T) {
	orig := originatingInvoice(t, "PROD-A", mustQuantity(t, 1))
	r := fakeReader{docs: map[string]IssuedDocument{orig.Number.Format(): orig}}
	draft := ndDraftWithRef(t, "PROD-B", mustQuantity(t, 1)) // different product
	_, err := IssueSalesInvoice(draft, &draft.Series, fakeSigner{control: "1"}, "u",
		time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC), IssueOptions{Reader: r})
	if err == nil {
		t.Fatal("ND with new product code should be rejected")
	}
}

func TestDraftND_RejectsQuantityChange(t *testing.T) {
	orig := originatingInvoice(t, "PROD-A", mustQuantity(t, 2))
	r := fakeReader{docs: map[string]IssuedDocument{orig.Number.Format(): orig}}
	draft := ndDraftWithRef(t, "PROD-A", mustQuantity(t, 3)) // qty differs
	_, err := IssueSalesInvoice(draft, &draft.Series, fakeSigner{control: "1"}, "u",
		time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC), IssueOptions{Reader: r})
	if err == nil {
		t.Fatal("ND with quantity change should be rejected")
	}
}

func TestDraftND_AcceptsValueAdjustmentSameProduct(t *testing.T) {
	orig := originatingInvoice(t, "PROD-A", mustQuantity(t, 2))
	r := fakeReader{docs: map[string]IssuedDocument{orig.Number.Format(): orig}}
	draft := ndDraftWithRef(t, "PROD-A", mustQuantity(t, 2)) // qty matches
	_, err := IssueSalesInvoice(draft, &draft.Series, fakeSigner{control: "1"}, "u",
		time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC), IssueOptions{Reader: r})
	if err != nil {
		t.Fatalf("ND matching product+qty should pass: %v", err)
	}
}

func TestDraftND_RequiresReader(t *testing.T) {
	draft := ndDraftWithRef(t, "PROD-A", mustQuantity(t, 1))
	_, err := IssueSalesInvoice(draft, &draft.Series, fakeSigner{control: "1"}, "u",
		time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err == nil {
		t.Fatal("ND without Reader should be rejected")
	}
}

func TestDraftFS_OverLimitRejected(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = FS
	d.Customer = NewAnonymousCustomer()
	s := registeredSeries(t, "FS1", FS)
	d.Series = *s
	// validDraft line = €100 * 1 = NetTotal 100 → Gross 123. Set a tighter Default
	// limit (€50) to force the cap to fire.
	limits := &FSLimits{Retail: Money(1000 * scale), Default: Money(50 * scale)}
	_, err := IssueSalesInvoice(&DraftSalesInvoice{CommonDraftDocument: *d}, s,
		fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
		IssueOptions{FSLimits: limits, IssuerEAC: "00000"}) // non-retail EAC
	if err == nil {
		t.Fatal("FS over default limit should be rejected")
	}
}

func TestDraftFS_AnonymousRetailUnderLimit(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = FS
	d.Customer = NewAnonymousCustomer()
	// Tag the line as goods so the retail tier resolves.
	d.Lines[0].Product.ProductType = ProductTypeGoods
	s := registeredSeries(t, "FS1", FS)
	d.Series = *s
	_, err := IssueSalesInvoice(&DraftSalesInvoice{CommonDraftDocument: *d}, s,
		fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
		IssueOptions{IssuerEAC: "47110"}) // retail EAC ⇒ €1000 tier ⇒ €123 < €1000
	if err != nil {
		t.Fatalf("retail anonymous FS under limit should pass: %v", err)
	}
}

func TestEAC_RetailActivity(t *testing.T) {
	if !IsRetailActivity("47110") {
		t.Errorf("47110 should be retail")
	}
	if IsRetailActivity("12345") {
		t.Errorf("12345 should not be retail")
	}
}

func TestWorkDocument_MarkBilled(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = OR
	s := registeredSeries(t, "W1", OR)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	wd, err := IssueWorkDocument(&DraftWorkDocument{CommonDraftDocument: *d}, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	invoiceRef, _ := NewDocNumber(FT, "A", 42)
	if err := wd.MarkBilled(invoiceRef, time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MarkBilled: %v", err)
	}
	if wd.Status != StatusBilled {
		t.Errorf("status: got %q, want %q", wd.Status, StatusBilled)
	}
	if wd.BilledByInvoice == nil || wd.BilledByInvoice.Format() != "FT A/42" {
		t.Errorf("BilledByInvoice not set correctly, got %+v", wd.BilledByInvoice)
	}
	if wd.Reason != "" {
		t.Errorf("Reason must not be used for billing link, got %q", wd.Reason)
	}
	// Re-billing must error.
	if err := wd.MarkBilled(invoiceRef, time.Now()); err == nil {
		t.Error("re-billing should be rejected")
	}
}

func TestIntegrateRecoveredDocument_BypassesGuards(t *testing.T) {
	// Replicate the AT exercise: manual series F document emitted 2026-01-02,
	// integrated months later out of normal sequence.
	d := validDraft(t)
	d.Date = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	d.Lines[0].TaxPointDate = d.Date
	s := registeredSeries(t, "F", FT)
	d.Series = *s
	now := time.Date(2026, 5, 22, 14, 0, 0, 0, time.UTC) // months later — past 5-day guard
	inv, err := IntegrateRecoveredDocument(&DraftSalesInvoice{CommonDraftDocument: *d}, s, fakeSigner{control: "1"}, "u", now)
	if err != nil {
		t.Fatalf("manual integration: %v", err)
	}
	if inv.SourceBilling != SourceBillingManual {
		t.Errorf("SourceBilling: got %q, want %q", inv.SourceBilling, SourceBillingManual)
	}
}

func TestIntegrateRecoveredDocument_AllowsBackdatedAfterRecoveredEntry(t *testing.T) {
	// AT exercise: series D #3 (2026-01-01) ingested AFTER series D #2 (2026-01-15)
	// would have advanced the monotonic-date marker. Recovery must accept it.
	s := registeredSeries(t, "D", FT)
	signer := fakeSigner{control: "1"}

	advance := validDraft(t)
	advance.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	advance.Lines[0].TaxPointDate = advance.Date
	advance.Series = *s
	if _, err := IntegrateRecoveredDocument(&DraftSalesInvoice{CommonDraftDocument: *advance}, s, signer, "u",
		time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	older := validDraft(t)
	older.Date = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older.Lines[0].TaxPointDate = older.Date
	older.Series = *s
	if _, err := IntegrateRecoveredDocument(&DraftSalesInvoice{CommonDraftDocument: *older}, s, signer, "u",
		time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("backup integration of older doc should succeed: %v", err)
	}
}

func TestSalesInvoiceMovementTimeOrder(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 17, 8, 0, 0, 0, time.UTC)
	end := start.Add(-time.Hour) // end before start
	draft := &DraftSalesInvoice{
		CommonDraftDocument: *d,
		SalesInvoiceFields: SalesInvoiceFields{
			MovementStartTime: &start,
			MovementEndTime:   &end,
		},
	}
	if _, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{}); err == nil {
		t.Fatal("expected error for movement end < start")
	}
}

func TestIssueSalesInvoice_SelfBillingPropagatesStatus(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftSalesInvoice{
		CommonDraftDocument: *d,
		SalesInvoiceFields:  SalesInvoiceFields{SpecialRegimes: SpecialRegimes{SelfBilling: true}},
	}
	inv, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != StatusSelfBilled {
		t.Errorf("Status: got %q, want %q", inv.Status, StatusSelfBilled)
	}
}

func TestIssue_SourceBillingHonored(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	inv, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{SourceBilling: SourceBillingIntegrated})
	if err != nil {
		t.Fatal(err)
	}
	if inv.SourceBilling != SourceBillingIntegrated {
		t.Errorf("SourceBilling: got %q, want %q", inv.SourceBilling, SourceBillingIntegrated)
	}
}

func TestIssue_DefaultSourceBillingIsProduced(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	inv, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if inv.SourceBilling != SourceBillingProduced {
		t.Errorf("SourceBilling: got %q, want %q", inv.SourceBilling, SourceBillingProduced)
	}
}

func TestIssue_RejectsInvalidSourceBilling(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	draft := &DraftSalesInvoice{CommonDraftDocument: *d}
	_, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{SourceBilling: "Z"})
	if err == nil {
		t.Fatal("expected error for invalid SourceBilling")
	}
}

// --- StockMovement ---

func transportDraft(t *testing.T) *CommonDraftDocument {
	t.Helper()
	d := validDraft(t)
	d.DocumentType = GT
	return d
}

// defaultShipPoints returns ShipFrom and ShipTo fixtures that satisfy
// DraftStockMovement.Validate (mandatory per P2.7).
func defaultShipPoints(t *testing.T) (*ShippingPoint, *ShippingPoint) {
	t.Helper()
	from, _ := NewAddress("Rua Origem 1", "Lisboa", "1000-100", "PT")
	to, _ := NewAddress("Rua Destino 2", "Porto", "4000-100", "PT")
	return &ShippingPoint{Address: &from}, &ShippingPoint{Address: &to}
}

func TestIssueStockMovementGT(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC)
	from, to := defaultShipPoints(t)
	draft := &DraftStockMovement{CommonDraftDocument: *d, StockMovementFields: StockMovementFields{MovementStartTime: start, ShipFrom: from, ShipTo: to}}
	sm, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !sm.DocumentType.IsTransport() {
		t.Errorf("doctype: %s", sm.DocumentType)
	}
	if !sm.MovementStartTime.Equal(start) {
		t.Errorf("MovementStartTime mismatch")
	}
}

func TestStockMovementRejectsStampDutyLine(t *testing.T) {
	d := transportDraft(t)
	stampTax, _ := NewStampLineTax("PT", "IS-G", mustMoney(t, 5))
	d.Lines[0].Tax = stampTax
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	from, to := defaultShipPoints(t)
	draft := &DraftStockMovement{CommonDraftDocument: *d, StockMovementFields: StockMovementFields{MovementStartTime: time.Now(), ShipFrom: from, ShipTo: to}}
	_, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected error for stamp duty on stock movement")
	}
}

func TestDraftStockMovement_MissingShipFromRejected(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	_, to := defaultShipPoints(t)
	// ShipFrom omitted
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{
			MovementStartTime: time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC),
			ShipTo:            to,
		},
	}
	_, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected ship_from required error")
	}
}

func TestDraftStockMovement_MissingShipToRejected(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	from, _ := defaultShipPoints(t)
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{
			MovementStartTime: time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC),
			ShipFrom:          from,
		},
	}
	_, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected ship_to required error")
	}
}

func TestStockMovement_NonValuedNoTax(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	// Strip price + tax from every line — pure non-valued guia.
	for i := range d.Lines {
		d.Lines[i].UnitPrice = 0
		d.Lines[i].Tax = nil
	}
	from, to := defaultShipPoints(t)
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC)
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{MovementStartTime: start, ShipFrom: from, ShipTo: to},
	}
	if _, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{}); err != nil {
		t.Fatalf("non-valued guia should pass: %v", err)
	}
}

func TestStockMovement_ValuedRequiresTax(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	// One priced line, one un-taxed line — must reject.
	d.Lines[0].Tax = nil // first line: UnitPrice=100, Tax=nil
	from, to := defaultShipPoints(t)
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC)
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{MovementStartTime: start, ShipFrom: from, ShipTo: to},
	}
	if _, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{}); err == nil {
		t.Fatal("valued guia with missing line tax should error")
	}
}

func TestIssueStockMovement_RejectsStartBeforeSystemEntry(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	from, to := defaultShipPoints(t)
	now := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 16, 9, 0, 0, 0, time.UTC) // 3h before system entry
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{MovementStartTime: start, ShipFrom: from, ShipTo: to},
	}
	_, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err == nil {
		t.Fatal("expected error when MovementStartTime precedes SystemEntryDate")
	}
}

func TestStockMovementRejectsZeroStartTime(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	draft := &DraftStockMovement{CommonDraftDocument: *d} // zero MovementStartTime
	_, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected error for zero MovementStartTime")
	}
}

func TestIssueStockMovement_ThirdPartiesPropagatesStatus(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC)
	from, to := defaultShipPoints(t)
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{MovementStartTime: start, ShipFrom: from, ShipTo: to, ThirdParties: true},
	}
	sm, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if sm.Status != StatusThirdParty {
		t.Errorf("Status: got %q, want %q", sm.Status, StatusThirdParty)
	}
}

// --- WorkDocument ---

func TestIssueWorkDocumentOR(t *testing.T) {
	d := validDraft(t)
	d.DocumentType = OR
	s := registeredSeries(t, "W1", OR)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	draft := &DraftWorkDocument{CommonDraftDocument: *d}
	wd, err := IssueWorkDocument(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !wd.DocumentType.IsWorking() {
		t.Errorf("doctype: %s", wd.DocumentType)
	}
}

func TestIssueWorkDocumentRejectsNonWorkingType(t *testing.T) {
	d := validDraft(t) // FT
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	draft := &DraftWorkDocument{CommonDraftDocument: *d}
	_, err := IssueWorkDocument(draft, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected error for non-working doc type")
	}
}

// --- Payment ---

func validPaymentDraft(t *testing.T) *PaymentDraft {
	t.Helper()
	addr, _ := NewAddress("Rua A 1", "Lisboa", "1000-001", "PT")
	cust, err := NewCustomer("ACC1", "503504564", "Acme", addr, false)
	if err != nil {
		t.Fatal(err)
	}
	credit, _ := NewMoney(100)
	return &PaymentDraft{
		Type:            RG,
		TransactionDate: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
		Customer:        *cust,
		SourceID:        "u",
		Lines: []PaymentLine{
			{
				LineNumber: 1,
				SourceDocuments: []SourceDocumentID{
					{OriginatingON: "FT A/1", InvoiceDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
				},
				Movement: CreditAmount{Value: credit},
			},
		},
	}
}

func TestIssuePaymentRG(t *testing.T) {
	d := validPaymentDraft(t)
	s := registeredSeries(t, "RG1", RG)
	now := time.Date(2026, 1, 17, 9, 0, 0, 0, time.UTC)
	net, _ := NewMoney(100)
	gross, _ := NewMoney(100)
	p, err := IssuePayment(d, s, "u", now, PaymentTotals{NetTotal: net, GrossTotal: gross}, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Number.Format() != "RG RG1/1" {
		t.Errorf("number: %s", p.Number.Format())
	}
	if p.Status != StatusNormal {
		t.Errorf("status: %s", p.Status)
	}
	if s.LastNum != 1 {
		t.Errorf("series LastNum: %d", s.LastNum)
	}
}

func TestIssuePaymentRCRequiresLineTax(t *testing.T) {
	d := validPaymentDraft(t)
	d.Type = RC
	// Lines have no Tax — should fail.
	s := registeredSeries(t, "RC1", RC)
	_, err := IssuePayment(d, s, "u", time.Now(), PaymentTotals{}, IssueOptions{})
	if err == nil {
		t.Fatal("expected error for RC without line Tax")
	}
}

func TestIssuePaymentRCWithLineTaxPasses(t *testing.T) {
	d := validPaymentDraft(t)
	d.Type = RC
	tax, _ := NewVATLineTax(PT, TaxNormal, "", "")
	d.Lines[0].Tax = tax
	s := registeredSeries(t, "RC1", RC)
	now := time.Date(2026, 1, 17, 9, 0, 0, 0, time.UTC)
	net, _ := NewMoney(100)
	gross, _ := NewMoney(123)
	tp, _ := NewMoney(23)
	if _, err := IssuePayment(d, s, "u", now, PaymentTotals{NetTotal: net, TaxPayable: tp, GrossTotal: gross}, IssueOptions{}); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestPayment_CurrencyDateMustMatchTransactionDate(t *testing.T) {
	d := validPaymentDraft(t)
	rate, _ := NewExchangeRate(1.085)
	amt, _ := NewMoney(100)
	wrongDay := d.TransactionDate.AddDate(0, 0, 1)
	bad := Currency{Code: "USD", Amount: amt, ExchangeRate: rate, Date: wrongDay}
	d.Currency = &bad
	s := registeredSeries(t, "RG1", RG)
	now := time.Date(2026, 1, 17, 9, 0, 0, 0, time.UTC)
	net, _ := NewMoney(100)
	gross, _ := NewMoney(100)
	_, err := IssuePayment(d, s, "u", now, PaymentTotals{NetTotal: net, GrossTotal: gross}, IssueOptions{})
	if err == nil {
		t.Fatal("expected error when Currency.Date != TransactionDate")
	}
}

func TestPaymentLineRequiresMovement(t *testing.T) {
	noMovement := PaymentLine{
		LineNumber:      1,
		SourceDocuments: []SourceDocumentID{{OriginatingON: "FT A/1", InvoiceDate: time.Now()}},
	}
	if err := noMovement.Validate(); err == nil {
		t.Fatal("PaymentLine without Movement: expected error")
	}
}

func TestDocumentType_DCRejected(t *testing.T) {
	if DocumentType("DC").IsValid() {
		t.Fatal("DC must not be a valid document type (removed per F-NEW-2)")
	}
}

func TestPaymentMechanismIsValid(t *testing.T) {
	for _, m := range []PaymentMechanism{
		PaymentMechanismCreditCard, PaymentMechanismCash, PaymentMechanismMultibanco, PaymentMechanismBankTransfer,
	} {
		if !m.IsValid() {
			t.Errorf("%s should be valid", m)
		}
	}
	if PaymentMechanism("ZZ").IsValid() {
		t.Error("ZZ should not be valid")
	}
}

func TestReceiptTypes(t *testing.T) {
	if !RC.IsReceipt() || !RG.IsReceipt() {
		t.Error("RC and RG should be receipt doc types")
	}
	if FT.IsReceipt() {
		t.Error("FT should not be a receipt")
	}
	if DocumentType("RX").IsValid() {
		t.Error("RX should not be a valid doc type")
	}
}

// Sanity: PaymentDraft Validate rejects zero customer.
func TestPaymentDraftRejectsMissingCustomer(t *testing.T) {
	d := validPaymentDraft(t)
	d.Customer = Customer{}
	d.Customer.CustomerID = uuid.Nil
	if err := d.Validate(); err == nil {
		t.Fatal("expected error for missing customer")
	}
}
