package app_test

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestMarkWorkBilled(t *testing.T) {
	svc, tenant := newTestServices(t)
	ctx := context.Background()

	// Issue a PF work document.
	pfIn := app.IssueWorkInput{
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
			ProductCode:        "SRV001",
			ProductType:        app.ProductService,
			ProductDescription: "Serviço faturado",
			ProductNumberCode:  "SRV001",
			Unit:               app.UnitHour,
			Quantity:           2,
			UnitPriceCents:     5000,
			TaxPointDate:       "2026-05-01",
			Tax: &app.LineTaxInput{
				Kind:     "VAT",
				Region:   app.RegionPT,
				Category: app.RateNormal,
			},
		}},
		Idem: app.IdempotencyKey{Key: "pf-markbilled-001", Fingerprint: "fp-pf-001"},
	}
	pf, err := svc.Invoicing.IssueWorkDocument(ctx, tenant, pfIn)
	if err != nil {
		t.Fatalf("IssueWorkDocument: %v", err)
	}

	// Issue an FT to bill against.
	ftIn := sampleFTInput()
	ft, err := svc.Invoicing.IssueInvoice(ctx, tenant, ftIn)
	if err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}

	// Mark the PF as billed by the FT.
	view, err := svc.Invoicing.MarkWorkBilled(ctx, tenant, pf.Number, ft.Number)
	if err != nil {
		t.Fatalf("MarkWorkBilled: %v", err)
	}
	if view.Status != "F" {
		t.Fatalf("expected Status=%q, got %q", "F", view.Status)
	}
}

func TestCancelDocument(t *testing.T) {
	svc, tenant := newTestServices(t)
	ctx := context.Background()

	// Issue an FT.
	ft, err := svc.Invoicing.IssueInvoice(ctx, tenant, sampleFTInput())
	if err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}

	// Cancel it.
	view, err := svc.Invoicing.CancelDocument(ctx, tenant, ft.Number, "Anulação a pedido")
	if err != nil {
		t.Fatalf("CancelDocument: %v", err)
	}
	if view.Status != "A" {
		t.Fatalf("expected Status=%q, got %q", "A", view.Status)
	}
}
