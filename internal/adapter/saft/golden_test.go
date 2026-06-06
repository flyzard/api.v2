package saft

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

var update = flag.Bool("update", false, "rewrite golden files")

// goldenDocs builds one representative document per family, exercising the
// fields the structural tests don't pin element-by-element: currency,
// settlement (global discount + payment terms), withholding, FR payments,
// shipping points, payment debit/credit lines, and a cancelled document per
// aggregate rule.
func goldenDocs() (sales []domain.SalesInvoice, stock []domain.StockMovement, work []domain.WorkDocument, payments []domain.Payment) {
	date := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
	due := date.AddDate(0, 0, 30)

	// FT with line + global discount shares, payment terms, FX currency, withholding.
	ft := minimalSalesInvoice()
	ft.PaymentTerms = &due
	ft.Lines[0].GlobalDiscountShare = must(domain.NewMoney(3.00))
	ft.Currency = &domain.Currency{
		Code:         "USD",
		Amount:       must(domain.NewMoney(123.00)),
		ExchangeRate: must(domain.NewExchangeRate(1.085)),
		Date:         date,
	}
	ft.WithholdingTax = []domain.WithholdingTax{{
		Type:        domain.WithholdingIRS,
		Description: "Retenção IRS",
		Amount:      must(domain.NewMoney(11.50)),
	}}

	// Cancelled NC — listed, counted, excluded from debit/credit sums.
	nc := minimalSalesInvoice()
	nc.Number = must(domain.NewDocNumber(domain.NC, "NC2026", 1))
	nc.DocumentType = domain.NC
	nc.ATCUD = "AAAAAAAA-2"
	nc.Status = domain.StatusCancelled
	nc.Reason = "Emitida por engano"

	// GT with shipping points and movement window.
	end := date.Add(6 * time.Hour)
	gt := domain.StockMovement{
		IssuedDocument: minimalSalesInvoice().IssuedDocument,
		StockMovementFields: domain.StockMovementFields{
			MovementStartTime: date.Add(2 * time.Hour),
			MovementEndTime:   &end,
			ATDocCodeID:       "ATDC12345",
			ShipFrom: &domain.ShippingPoint{Address: &domain.Address{
				AddressDetail: "Armazem 1", City: "Lisboa", PostalCode: "1000-001", Country: "PT",
			}},
			ShipTo: &domain.ShippingPoint{Address: &domain.Address{
				AddressDetail: "Loja 9", City: "Porto", PostalCode: "4000-002", Country: "PT",
			}},
		},
	}
	gt.Number = must(domain.NewDocNumber(domain.GT, "GT2026", 1))
	gt.DocumentType = domain.GT
	gt.ATCUD = "AAAAAAAA-3"

	// PF working document.
	pf := domain.WorkDocument{IssuedDocument: minimalSalesInvoice().IssuedDocument}
	pf.Number = must(domain.NewDocNumber(domain.PF, "PF2026", 1))
	pf.DocumentType = domain.PF
	pf.ATCUD = "AAAAAAAA-4"

	// RG with credit + debit lines, method, withholding.
	settle := must(domain.NewMoney(1.00))
	rg := domain.Payment{
		Number:          must(domain.NewDocNumber(domain.RG, "RG2026", 1)),
		ATCUD:           "AAAAAAAA-5",
		TransactionDate: date,
		Type:            domain.RG,
		Status:          domain.StatusNormal,
		StatusDate:      date,
		SourcePayment:   domain.SourceBillingProduced,
		SourceID:        "issuer@test",
		SystemEntryDate: date,
		Customer:        minimalSalesInvoice().Customer,
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismCash,
			Amount:    must(domain.NewMoney(100.00)),
			Date:      date,
		}},
		Lines: []domain.PaymentLine{
			{
				LineNumber: 1,
				SourceDocuments: []domain.SourceDocumentID{{
					OriginatingON: "FT FT2026/1",
					InvoiceDate:   date,
					Description:   "Liquidação parcial",
				}},
				SettlementAmount: &settle,
				Movement:         domain.CreditAmount{Value: must(domain.NewMoney(110.00))},
			},
			{
				LineNumber: 2,
				SourceDocuments: []domain.SourceDocumentID{{
					OriginatingON: "NC NC2026/1",
					InvoiceDate:   date,
				}},
				Movement: domain.DebitAmount{Value: must(domain.NewMoney(10.00))},
			},
		},
		PaymentTotals: domain.PaymentTotals{
			NetTotal:   must(domain.NewMoney(100.00)),
			TaxPayable: must(domain.NewMoney(0.00)),
			GrossTotal: must(domain.NewMoney(100.00)),
		},
		WithholdingTax: []domain.WithholdingTax{{
			Type:        domain.WithholdingIRS,
			Description: "Retenção IRS",
			Amount:      must(domain.NewMoney(5.00)),
		}},
	}

	// Cancelled RG — counted in NumberOfEntries, excluded from debit/credit sums.
	rgCancelled := rg
	rgCancelled.Number = must(domain.NewDocNumber(domain.RG, "RG2026", 2))
	rgCancelled.ATCUD = "AAAAAAAA-6"
	rgCancelled.Status = domain.StatusCancelled
	rgCancelled.Reason = "Recebido em duplicado"

	return []domain.SalesInvoice{ft, nc},
		[]domain.StockMovement{gt},
		[]domain.WorkDocument{pf},
		[]domain.Payment{rg, rgCancelled}
}

// TestExport_Golden pins the full export byte-for-byte. The projector output
// is regulatory record-of-truth — any refactor must keep these bytes identical
// unless it deliberately fixes a spec violation, in which case regenerate with
//
//	go test ./internal/adapter/saft -run TestExport_Golden -update
//
// and review the golden diff like production code.
func TestExport_Golden(t *testing.T) {
	hdr := gdTestHeader()
	hdr.Issuer.EACCode = "47190"

	sales, stock, work, payments := goldenDocs()
	out, err := Export(hdr, sales, stock, work, payments)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	golden := filepath.Join("testdata", "golden_export.xml")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, out, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden rewritten: %s (%d bytes)", golden, len(out))
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(out, want) {
		// Locate first divergence for a usable failure message.
		i := 0
		for i < len(out) && i < len(want) && out[i] == want[i] {
			i++
		}
		lo := max(0, i-80)
		t.Fatalf("export differs from golden at byte %d:\n got: …%q\nwant: …%q",
			i, out[lo:min(len(out), i+80)], want[lo:min(len(want), i+80)])
	}
}
