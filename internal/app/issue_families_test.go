package app_test

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestIssueFamilies(t *testing.T) {
	svc, tenant := newTestServices(t)
	ctx := context.Background()

	t.Run("work_doc_OR", func(t *testing.T) {
		in := app.IssueWorkInput{
			DocType:  app.DocOR,
			SeriesID: "OR2026",
			SourceID: "SRC-WD-001",
			IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
			Date:     "2026-05-01",
			Customer: app.CustomerInput{
				TaxID:   "502819472",
				Name:    "Restaurante O Cantinho, Lda",
				Country: "PT",
				Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
			},
			Lines: []app.LineInput{{
				ProductCode:        "SRV001",
				ProductType:        app.ProductService,
				ProductDescription: "Orçamento de serviço",
				ProductNumberCode:  "SRV001",
				Unit:               app.UnitHour,
				Quantity:           2,
				UnitPriceCents:     5000, // 50.00 EUR/h
				TaxPointDate:       "2026-05-01",
				Tax: &app.LineTaxInput{
					Kind:     "VAT",
					Region:   app.RegionPT,
					Category: app.RateNormal,
				},
			}},
			Idem: app.IdempotencyKey{Key: "or-family-001", Fingerprint: "fp-or-001"},
		}
		view, err := svc.Invoicing.IssueWorkDocument(ctx, tenant, in)
		if err != nil {
			t.Fatalf("IssueWorkDocument: %v", err)
		}
		if view.GrossCents == 0 {
			t.Fatalf("expected non-zero GrossCents, got %d", view.GrossCents)
		}
		if view.Number == "" {
			t.Fatalf("expected non-empty Number")
		}
	})

	t.Run("stock_movement_GT", func(t *testing.T) {
		in := app.IssueStockInput{
			DocType:  app.DocGT,
			SeriesID: "GT2026",
			SourceID: "SRC-GT-001",
			IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
			Date:     "2026-05-01",
			Customer: app.CustomerInput{
				TaxID:   "502819472",
				Name:    "Restaurante O Cantinho, Lda",
				Country: "PT",
				Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
			},
			Lines: []app.LineInput{{
				ProductCode:        "PROD001",
				ProductType:        app.ProductGoods,
				ProductDescription: "Mercadoria em trânsito",
				ProductNumberCode:  "PROD001",
				Unit:               app.UnitPiece,
				Quantity:           10,
				UnitPriceCents:     2000, // 20.00 EUR each
				TaxPointDate:       "2026-05-01",
				Tax: &app.LineTaxInput{
					Kind:     "VAT",
					Region:   app.RegionPT,
					Category: app.RateNormal,
				},
			}},
			ShipFrom:          &app.AddressInput{Detail: "Rua do Armazém, 5", City: "Lisboa", PostalCode: "1000-001"},
			ShipTo:            &app.AddressInput{Detail: "Rua de Entrega, 10", City: "Porto", PostalCode: "4000-001"},
			MovementStartTime: "2026-05-01T12:00:00Z",
			Idem:              app.IdempotencyKey{Key: "gt-family-001", Fingerprint: "fp-gt-001"},
		}
		view, err := svc.Invoicing.IssueStockMovement(ctx, tenant, in)
		if err != nil {
			t.Fatalf("IssueStockMovement: %v", err)
		}
		if view.GrossCents == 0 {
			t.Fatalf("expected non-zero GrossCents, got %d", view.GrossCents)
		}
		if view.Number == "" {
			t.Fatalf("expected non-empty Number")
		}
	})

	t.Run("payment_RG_advance", func(t *testing.T) {
		const advanceCents = int64(50000) // 500.00 EUR advance
		in := app.IssuePaymentInput{
			Type:            app.DocRG,
			SeriesID:        "RG2026",
			TransactionDate: "2026-05-01",
			Customer: app.CustomerInput{
				TaxID:   "502819472",
				Name:    "Restaurante O Cantinho, Lda",
				Country: "PT",
				Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
			},
			SourceID: "SRC-RG-001",
			Methods: []app.PaymentMethodInput{{
				Mechanism:   app.MechBankTransfer,
				AmountCents: advanceCents,
				Date:        "2026-05-01",
			}},
			Lines: []app.PaymentLineInput{{
				LineNumber: 1,
				SourceDocuments: []app.SourceDocInput{{
					OriginatingON: "Adiantamento 2026-06-16",
					InvoiceDate:   "2026-05-01",
					Description:   "Adiantamento sobre serviços futuros",
				}},
				CreditCents: advanceCents,
			}},
			Totals: app.TotalsInput{
				NetCents:   advanceCents,
				TaxCents:   0,
				GrossCents: advanceCents,
			},
			Idem: app.IdempotencyKey{Key: "rg-family-001", Fingerprint: "fp-rg-001"},
		}
		view, err := svc.Invoicing.IssuePayment(ctx, tenant, in)
		if err != nil {
			t.Fatalf("IssuePayment: %v", err)
		}
		if view.GrossCents != advanceCents {
			t.Fatalf("expected GrossCents=%d, got %d", advanceCents, view.GrossCents)
		}
		if view.Number == "" {
			t.Fatalf("expected non-empty Number")
		}
	})
}
