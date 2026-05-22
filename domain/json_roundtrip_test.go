package domain

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

// assertRoundTrip marshals orig, unmarshals into a fresh value of type *T, and
// re-marshals the result. The two byte slices must be byte-identical — that
// pins both the wire format and the symmetry of Marshal/Unmarshal for the type.
// Go's json.Marshal is deterministic for structs (field-declaration order) and
// for string-keyed maps (sorted), which is what every type in this package uses.
func assertRoundTrip[T any](t *testing.T, orig any, dst *T) {
	t.Helper()
	first, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal #1: %v", err)
	}
	if err := json.Unmarshal(first, dst); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, first)
	}
	second, err := json.Marshal(dst)
	if err != nil {
		t.Fatalf("marshal #2: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("re-marshal differs.\n  first:  %s\n  second: %s", first, second)
	}
}

func TestSalesInvoiceJSONRoundTrip(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)

	draft := &DraftSalesInvoice{
		CommonDraftDocument: *d,
		SalesInvoiceFields: SalesInvoiceFields{
			SpecialRegimes: SpecialRegimes{SelfBilling: true, CashVAT: false},
			WithholdingTax: []WithholdingTax{
				{Type: WithholdingIRS, Description: "IRS retention", Amount: mustMoney(t, 5)},
			},
		},
	}
	inv, err := IssueSalesInvoice(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var back SalesInvoice
	assertRoundTrip(t, inv, &back)
}

func TestStockMovementJSONRoundTrip(t *testing.T) {
	d := transportDraft(t)
	s := registeredSeries(t, "T1", GT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 16, 11, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	from, to := defaultShipPoints(t)
	draft := &DraftStockMovement{
		CommonDraftDocument: *d,
		StockMovementFields: StockMovementFields{
			MovementStartTime: start,
			MovementEndTime:   &end,
			ATDocCodeID:       "AT-CODE-123",
			ShipFrom:          from,
			ShipTo:            to,
		},
	}
	sm, err := IssueStockMovement(draft, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var back StockMovement
	assertRoundTrip(t, sm, &back)
}

func TestWorkDocumentJSONRoundTrip(t *testing.T) {
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

	var back WorkDocument
	assertRoundTrip(t, wd, &back)
}

func TestPaymentJSONRoundTrip(t *testing.T) {
	d := validPaymentDraft(t)
	s := registeredSeries(t, "RG1", RG)
	now := time.Date(2026, 1, 17, 9, 0, 0, 0, time.UTC)
	net, _ := NewMoney(100)
	gross, _ := NewMoney(100)
	p, err := IssuePayment(d, s, "u", now, PaymentTotals{NetTotal: net, GrossTotal: gross}, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var back Payment
	assertRoundTrip(t, p, &back)
}
