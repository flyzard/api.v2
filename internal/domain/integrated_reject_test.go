package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueSalesInvoice_RejectsIntegrated(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	series := mustVal(NewSeries("A2026", FT))
	if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
		t.Fatal(err)
	}
	before := series
	d := &DraftSalesInvoice{}
	d.DocumentType = FT
	d.Customer = Customer{
		CustomerID:     uuid.New(),
		AccountID:      "ACC-PT",
		CustomerTaxID:  "500000000",
		CompanyName:    "X",
		BillingAddress: Address{AddressDetail: "R", City: "L", PostalCode: "1000-001", Country: "PT"},
	}
	d.Date = now
	d.Series = series
	d.Lines = []DocumentLine{normalVATLine(now)}
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}
	_, err := IssueSalesInvoice(d, &series, m16StubSigner{}, "t", now, IssueOptions{SourceBilling: SourceBillingIntegrated}, qr)
	if !errors.Is(err, ErrIntegratedNotSupported) {
		t.Fatalf("err = %v, want ErrIntegratedNotSupported", err)
	}
	if series.LastNum != before.LastNum || series.LastHash != before.LastHash {
		t.Error("rejected I-issuance must not advance the series")
	}
}

func TestIssuePayment_RejectsIntegrated(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	series := mustVal(NewSeries("RG2026", RG))
	if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
		t.Fatal(err)
	}
	before := series
	draft := &PaymentDraft{
		Type:            RG,
		TransactionDate: now,
		Customer: Customer{
			CustomerID:     uuid.New(),
			AccountID:      "ACC-RG",
			CustomerTaxID:  "500000000",
			CompanyName:    "X",
			BillingAddress: Address{AddressDetail: "R", City: "L", PostalCode: "1000-001", Country: "PT"},
		},
		SourceID: "t",
		Lines: []PaymentLine{{
			LineNumber: 1,
			SourceDocuments: []SourceDocumentID{{
				OriginatingON: "Adiantamento",
				InvoiceDate:   now,
			}},
			Movement: CreditAmount{Value: Money(1000)},
		}},
	}
	totals := PaymentTotals{NetTotal: Money(1000), GrossTotal: Money(1000)}
	_, err := IssuePayment(draft, &series, now, totals, IssueOptions{SourceBilling: SourceBillingIntegrated})
	if !errors.Is(err, ErrIntegratedNotSupported) {
		t.Fatalf("err = %v, want ErrIntegratedNotSupported", err)
	}
	if series.LastNum != before.LastNum || series.LastHash != before.LastHash {
		t.Error("rejected I-payment-issuance must not advance the series")
	}
}
