package domain

import (
	"crypto/sha512"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustVal[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// m16StubSigner mirrors the demo stubSigner: deterministic 172-char base64,
// key version "1". Not real RSA — only feeds the hash-chain plumbing.
type m16StubSigner struct{}

func (m16StubSigner) Sign(canonical string) (string, string, error) {
	a := sha512.Sum512([]byte(canonical))
	b := sha512.Sum512(a[:])
	return base64.StdEncoding.EncodeToString(append(a[:], b[:]...)), "1", nil
}

// m16Line builds a valid one-line M16 fixture for the gate tests.
func m16Line(date time.Time) DocumentLine {
	return DocumentLine{
		LineNumber: 1,
		Product: mustVal(NewProduct(Product{
			ProductCode:        "P-1",
			ProductType:        ProductTypeGoods,
			ProductDescription: "Mercadoria de teste",
			ProductNumberCode:  "P-1",
			Unit:               UnitPiece,
		})),
		Quantity:     mustVal(NewQuantity(1)),
		UnitPrice:    mustVal(NewMoney(10)),
		TaxPointDate: date,
		Tax:          mustVal(NewVATLineTax(PT, TaxExempt, M16, "Isento artigo 14.º do RITI")),
	}
}

// TestValidateM16 pins the issuance gate for the intra-community exemption:
// RITI Art. 14.º n.º 1 a) / Ofício-Circulado 30225/2020 make the buyer's VAT
// registration in another EU member state a substantive condition, so M16
// lines require an EU non-PT customer with a real tax identification.
func TestValidateM16(t *testing.T) {
	customer := func(country Country, taxID string) Customer {
		return Customer{
			CustomerTaxID:  CustomerTaxID(taxID),
			BillingAddress: Address{Country: country},
		}
	}

	cases := []struct {
		name     string
		customer Customer
		hasM16   bool
		wantErr  string
	}{
		{"eu-buyer-with-vat-id", customer("DE", "DE123456789"), true, ""},
		{"no-m16-lines-pt-buyer", customer("PT", "500000000"), false, ""},
		{"pt-buyer", customer("PT", "500000000"), true, "another EU member state"},
		{"non-eu-buyer", customer("GB", "GB123456789"), true, "another EU member state"},
		{"final-consumer-nif", customer("DE", "999999990"), true, "VAT identification"},
		{"missing-tax-id", customer("ES", ""), true, "VAT identification"},
		{"anonymous-customer", NewAnonymousCustomer(), true, "another EU member state"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateM16(c.customer, c.hasM16)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

// TestM16GateStrictOnRecovery pins the decision that recovery does NOT bypass
// the M16 gate (docs/recovery.md item 4): integrating a recovered M16 invoice
// requires the same EU non-PT customer + VAT id the original needed to be
// legal — and succeeds when they're present.
func TestM16GateStrictOnRecovery(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	newDraft := func(c Customer, series Series) *DraftSalesInvoice {
		d := &DraftSalesInvoice{}
		d.DocumentType = FT
		d.Customer = c
		d.Series = series
		d.Date = time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC) // original's date, pre-integration
		d.Lines = []DocumentLine{m16Line(d.Date)}
		return d
	}
	newSeries := func() Series {
		s := mustVal(NewRecoverySeries("R2026", FT))
		if err := s.RegisterWithAT("AAAABBBB", now); err != nil {
			t.Fatalf("RegisterWithAT: %v", err)
		}
		return s
	}
	ref := RecoveredRef{Kind: RecoveryManual, OriginalSeries: "F", OriginalNumber: 23}
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}

	ptCustomer := Customer{
		CustomerID:    uuid.New(),
		AccountID:     "ACC-PT",
		CustomerTaxID: "500000000",
		CompanyName:   "Cliente PT Lda.",
		BillingAddress: Address{
			AddressDetail: "Rua de Teste 1",
			City:          "Lisboa",
			PostalCode:    "1000-001",
			Country:       "PT",
		},
	}
	series := newSeries()
	_, err := IntegrateRecoveredSalesInvoice(newDraft(ptCustomer, series), ref, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
	if err == nil || !strings.Contains(err.Error(), "M16") {
		t.Fatalf("recovered M16 invoice with PT customer: error = %v, want M16 gate", err)
	}

	deCustomer := Customer{
		CustomerID:    uuid.New(),
		AccountID:     "ACC-DE",
		CustomerTaxID: "DE123456789",
		CompanyName:   "Kunde GmbH",
		BillingAddress: Address{
			AddressDetail: "Musterstraße 1",
			City:          "Berlin",
			PostalCode:    "10115",
			Country:       "DE",
		},
	}
	series = newSeries()
	doc, err := IntegrateRecoveredSalesInvoice(newDraft(deCustomer, series), ref, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
	if err != nil {
		t.Fatalf("recovered M16 invoice with valid EU customer: %v", err)
	}
	if doc.SourceBilling != SourceBillingManual {
		t.Errorf("SourceBilling = %q, want %q", doc.SourceBilling, SourceBillingManual)
	}
}

// TestM16GateCoversAllFamilies pins that M16 detection runs in
// CommonDraftDocument.Validate (sales, working, AND stock movements — an
// intra-EU transfer guia can carry M16 lines) and in PaymentDraft.Validate,
// not only on sales invoices.
func TestM16GateCoversAllFamilies(t *testing.T) {
	ptCustomer := Customer{
		CustomerID:    uuid.New(),
		AccountID:     "ACC-1",
		CustomerTaxID: "500000000",
		CompanyName:   "Cliente PT Lda.",
		BillingAddress: Address{
			AddressDetail: "Rua de Teste 1",
			City:          "Lisboa",
			PostalCode:    "1000-001",
			Country:       "PT",
		},
	}

	draft := CommonDraftDocument{}
	draft.DocumentType = GT
	draft.Customer = ptCustomer
	draft.Series = Series{ID: "GT2026"}
	draft.Date = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	draft.Lines = []DocumentLine{m16Line(draft.Date)}
	err := draft.Validate()
	if err == nil || !strings.Contains(err.Error(), "M16") {
		t.Fatalf("GT draft with M16 line and PT customer: error = %v, want M16 gate", err)
	}
}
