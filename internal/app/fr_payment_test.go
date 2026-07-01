package app_test

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

// TestFRPaymentSumAtCentPrecision exercises the loosened FR guard:
// payment sum must match gross total at cent precision (not exact 5dp).
// Line: 2× €3.45 with 23% VAT → internal gross €8.487 → rounds to €8.49.
func TestFRPaymentSumAtCentPrecision(t *testing.T) {
	ctx := context.Background()
	frInput := func(amountCents int64, idemKey string) app.IssueInvoiceInput {
		return app.IssueInvoiceInput{
			DocType:  app.DocFR,
			SeriesID: "FR2026",
			SourceID: "SRC-FR-001",
			IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
			Date:     "2026-05-01",
			Customer: app.CustomerInput{
				TaxID:   "502819472",
				Name:    "Restaurante O Cantinho, Lda",
				Country: "PT",
				Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
			},
			Lines: []app.LineInput{{
				ProductCode:        "P001",
				ProductType:        app.ProductGoods,
				ProductDescription: "Produto sub-cent",
				ProductNumberCode:  "P001",
				Unit:               app.UnitPiece,
				Quantity:           2,
				UnitPriceCents:     345, // 2 × €3.45 = €6.90 net; 23% VAT = €1.587 → gross €8.487 → €8.49
				TaxPointDate:       "2026-05-01",
				Tax: &app.LineTaxInput{
					Kind:     "VAT",
					Region:   app.RegionPT,
					Category: app.RateNormal,
				},
			}},
			Payments: []app.FRPaymentInput{{
				Mechanism:   app.MechCash,
				AmountCents: amountCents,
				Date:        "2026-05-01",
			}},
			Idem: app.IdempotencyKey{Key: idemKey, Fingerprint: idemKey + "-fp"},
		}
	}

	t.Run("positive_exact_cent", func(t *testing.T) {
		svc, tenant := newTestServices(t)
		view, err := svc.Invoicing.IssueInvoice(ctx, tenant, frInput(849, "fr-cent-ok"))
		if err != nil {
			t.Fatalf("IssueInvoice FR with 849 cents: expected success, got %v", err)
		}
		if view.GrossCents != 849 {
			t.Fatalf("expected GrossCents=849, got %d", view.GrossCents)
		}
	})

	t.Run("negative_off_by_one_cent", func(t *testing.T) {
		svc, tenant := newTestServices(t)
		_, err := svc.Invoicing.IssueInvoice(ctx, tenant, frInput(850, "fr-cent-bad"))
		if err == nil {
			t.Fatal("IssueInvoice FR with 850 cents: expected error, got nil")
		}
		if app.KindOf(err) != app.KindInvalid {
			t.Fatalf("expected KindInvalid, got %v", app.KindOf(err))
		}
	})
}
