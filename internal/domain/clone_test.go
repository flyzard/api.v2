package domain

import (
	"testing"
	"time"
)

func TestIssueSalesInvoice_DoesNotAliasDraft(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := gdDraft(t, nil)
	series := mustVal(NewSeries("GD2026", FT))
	if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	d.Series = series
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}

	inv, err := IssueSalesInvoice(d, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
	if err != nil {
		t.Fatalf("IssueSalesInvoice: %v", err)
	}

	gross := inv.Totals.GrossTotal
	nLines := len(inv.Lines)
	origUnit := inv.Lines[0].UnitPrice

	// Mutate the draft AFTER issuance — the signed document must not move.
	d.Lines[0].UnitPrice = Money(999 * scale)
	d.Lines = append(d.Lines, d.Lines[0])

	if inv.Totals.GrossTotal != gross || len(inv.Lines) != nLines {
		t.Fatal("issued document changed when the draft was mutated")
	}
	if inv.Lines[0].UnitPrice != origUnit {
		t.Fatalf("issued line mutated via draft alias: %v", inv.Lines[0].UnitPrice)
	}
}

// Second-level pointees: cloning the slice/pointer alone still shares
// OrderDate, ShippingPoint internals, ShipToAddresses backing and payment
// settlement pointers with the draft. Writing through any of them after
// issuance must not move the issued document.
func TestIssue_DoesNotAliasNestedPointees(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}

	t.Run("sales invoice", func(t *testing.T) {
		d := gdDraft(t, nil)
		orderDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		d.Lines[0].OrderReferences = []OrderReference{{OriginatingON: "ENC 1/1", OrderDate: &orderDate}}
		d.Customer.ShipToAddresses = []Address{{
			AddressDetail: "Armazém 1", City: "Lisboa", PostalCode: "1000-001", Country: "PT",
		}}
		d.ShipFrom = &ShippingPoint{
			DeliveryIDs: []string{"DL-1"},
			Address: &Address{
				AddressDetail: "Cais 1", City: "Lisboa", PostalCode: "1000-002", Country: "PT",
			},
		}
		series := mustVal(NewSeries("NP2026", FT))
		if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
			t.Fatalf("RegisterWithAT: %v", err)
		}
		d.Series = series

		inv, err := IssueSalesInvoice(d, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
		if err != nil {
			t.Fatalf("IssueSalesInvoice: %v", err)
		}

		origOrderDate := orderDate
		*d.Lines[0].OrderReferences[0].OrderDate = orderDate.AddDate(1, 0, 0)
		d.Customer.ShipToAddresses[0].City = "Porto"
		d.ShipFrom.Address.City = "Faro"
		d.ShipFrom.DeliveryIDs[0] = "DL-MUT"

		if got := inv.Lines[0].OrderReferences[0].OrderDate; !got.Equal(origOrderDate) {
			t.Errorf("issued OrderDate mutated via draft alias: %v", got)
		}
		if got := inv.Customer.ShipToAddresses[0].City; got != "Lisboa" {
			t.Errorf("issued ShipToAddresses mutated via draft alias: %q", got)
		}
		if got := inv.ShipFrom.Address.City; got != "Lisboa" {
			t.Errorf("issued ShipFrom.Address mutated via draft alias: %q", got)
		}
		if got := inv.ShipFrom.DeliveryIDs[0]; got != "DL-1" {
			t.Errorf("issued ShipFrom.DeliveryIDs mutated via draft alias: %q", got)
		}
	})

	t.Run("payment", func(t *testing.T) {
		d := validPaymentDraft()
		settle := Money(10 * scale)
		d.Lines[0].SettlementAmount = &settle
		series := mustVal(NewSeries("NPRG2026", RG))
		if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
			t.Fatalf("RegisterWithAT: %v", err)
		}
		totals := PaymentTotals{NetTotal: Money(10 * scale), GrossTotal: Money(10 * scale)}

		p, err := IssuePayment(d, &series, now, totals, IssueOptions{})
		if err != nil {
			t.Fatalf("IssuePayment: %v", err)
		}

		*d.Lines[0].SettlementAmount = Money(999 * scale)
		d.Lines[0].SourceDocuments[0].OriginatingON = "MUT"

		if got := *p.Lines[0].SettlementAmount; got != Money(10*scale) {
			t.Errorf("issued SettlementAmount mutated via draft alias: %v", got)
		}
		if got := p.Lines[0].SourceDocuments[0].OriginatingON; got != "FT FT2026/1" {
			t.Errorf("issued SourceDocuments mutated via draft alias: %q", got)
		}
	})
}
