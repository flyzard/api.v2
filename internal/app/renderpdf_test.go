package app_test

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestRenderPDF(t *testing.T) {
	svc, tenant := newTestServices(t)
	ctx := context.Background()

	// Issue one of each family.

	// FT — sales invoice
	ftView, err := svc.Invoicing.IssueInvoice(ctx, tenant, sampleFTInput())
	if err != nil {
		t.Fatalf("IssueSalesInvoice: %v", err)
	}

	// PF — work document
	wdView, err := svc.Invoicing.IssueWorkDocument(ctx, tenant, app.IssueWorkInput{
		DocType:  app.DocPF,
		SeriesID: "PF2026",
		SourceID: "SRC-PF-001",
		IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
		Date:     "2026-05-01",
		Customer: app.CustomerInput{
			TaxID:   "502819472",
			Name:    "Restaurante O Cantinho, Lda",
			Country: "PT",
			Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
		},
		Lines: []app.LineInput{{
			ProductCode:        "SRV002",
			ProductType:        app.ProductService,
			ProductDescription: "Consulta técnica",
			ProductNumberCode:  "SRV002",
			Unit:               app.UnitHour,
			Quantity:           3,
			UnitPriceCents:     8000,
			TaxPointDate:       "2026-05-01",
			Tax: &app.LineTaxInput{
				Kind:     "VAT",
				Region:   app.RegionPT,
				Category: app.RateNormal,
			},
		}},
		Idem: app.IdempotencyKey{Key: "pf-render-001", Fingerprint: "fp-pf-render-001"},
	})
	if err != nil {
		t.Fatalf("IssueWorkDocument: %v", err)
	}

	// GT — stock movement
	gtView, err := svc.Invoicing.IssueStockMovement(ctx, tenant, app.IssueStockInput{
		DocType:  app.DocGT,
		SeriesID: "GT2026",
		SourceID: "SRC-GT-RENDER-001",
		IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
		Date:     "2026-05-01",
		Customer: app.CustomerInput{
			TaxID:   "502819472",
			Name:    "Restaurante O Cantinho, Lda",
			Country: "PT",
			Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
		},
		Lines: []app.LineInput{{
			ProductCode:        "PROD002",
			ProductType:        app.ProductGoods,
			ProductDescription: "Mercadoria em trânsito",
			ProductNumberCode:  "PROD002",
			Unit:               app.UnitPiece,
			Quantity:           5,
			UnitPriceCents:     3000,
			TaxPointDate:       "2026-05-01",
			Tax: &app.LineTaxInput{
				Kind:     "VAT",
				Region:   app.RegionPT,
				Category: app.RateNormal,
			},
		}},
		ShipFrom:          &app.AddressInput{Detail: "Rua do Armazém, 5", City: "Lisboa", PostalCode: "1000-001"},
		ShipTo:            &app.AddressInput{Detail: "Rua de Entrega, 10", City: "Porto", PostalCode: "4000-001"},
		MovementStartTime: "2026-05-01T10:00:00Z",
		Idem:              app.IdempotencyKey{Key: "gt-render-001", Fingerprint: "fp-gt-render-001"},
	})
	if err != nil {
		t.Fatalf("IssueStockMovement: %v", err)
	}

	// RG — payment/receipt
	const advanceCents = int64(20000)
	rgView, err := svc.Invoicing.IssuePayment(ctx, tenant, app.IssuePaymentInput{
		Type:            app.DocRG,
		SeriesID:        "RG2026",
		TransactionDate: "2026-05-01",
		Customer: app.CustomerInput{
			TaxID:   "502819472",
			Name:    "Restaurante O Cantinho, Lda",
			Country: "PT",
			Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
		},
		SourceID: "SRC-RG-RENDER-001",
		Methods: []app.PaymentMethodInput{{
			Mechanism:   app.MechBankTransfer,
			AmountCents: advanceCents,
			Date:        "2026-05-01",
		}},
		Lines: []app.PaymentLineInput{{
			LineNumber: 1,
			SourceDocuments: []app.SourceDocInput{{
				OriginatingON: "Adiantamento render",
				InvoiceDate:   "2026-05-01",
				Description:   "Teste",
			}},
			CreditCents: advanceCents,
		}},
		Totals: app.TotalsInput{
			NetCents:   advanceCents,
			TaxCents:   0,
			GrossCents: advanceCents,
		},
		Idem: app.IdempotencyKey{Key: "rg-render-001", Fingerprint: "fp-rg-render-001"},
	})
	if err != nil {
		t.Fatalf("IssuePayment: %v", err)
	}

	// Render each family and assert non-empty bytes.
	for _, tc := range []struct {
		name   string
		number string
	}{
		{"FT sales invoice", ftView.Number},
		{"PF work document", wdView.Number},
		{"GT stock movement", gtView.Number},
		{"RG payment", rgView.Number},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := svc.Query.RenderPDF(ctx, tenant, tc.number, app.Original)
			if err != nil {
				t.Fatalf("RenderPDF(%s): %v", tc.number, err)
			}
			if len(b) == 0 {
				t.Fatalf("RenderPDF(%s): got empty bytes", tc.number)
			}
		})
	}

	// RequiredVias: GT (transport) must have 4 vias; FT (sales) must have 2.
	t.Run("RequiredVias_GT", func(t *testing.T) {
		vias, err := app.RequiredVias(app.DocGT)
		if err != nil {
			t.Fatalf("RequiredVias(GT): %v", err)
		}
		if len(vias) != 4 {
			t.Fatalf("expected 4 vias for GT, got %d", len(vias))
		}
	})

	t.Run("RequiredVias_FT", func(t *testing.T) {
		vias, err := app.RequiredVias(app.DocFT)
		if err != nil {
			t.Fatalf("RequiredVias(FT): %v", err)
		}
		if len(vias) != 2 {
			t.Fatalf("expected 2 vias for FT, got %d", len(vias))
		}
	})

	t.Run("RenderPDF_unknown_copy_kind", func(t *testing.T) {
		_, err := svc.Query.RenderPDF(ctx, tenant, ftView.Number, app.CopyKind(99))
		if err == nil {
			t.Fatal("expected error for unknown CopyKind, got nil")
		}
		if app.KindOf(err) != app.KindInvalid {
			t.Fatalf("expected KindInvalid, got %v", app.KindOf(err))
		}
	})
}
