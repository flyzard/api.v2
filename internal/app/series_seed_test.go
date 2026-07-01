package app_test

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestSeedRegisteredSeries(t *testing.T) {
	svc, tenant := newTestServices(t)
	ctx := context.Background()

	// Seed a new series not already in the harness.
	if err := svc.Series.SeedRegisteredSeries(ctx, tenant, "FT2099", app.DocFT, "ATCODESEED1", "2026-04-01"); err != nil {
		t.Fatalf("SeedRegisteredSeries: %v", err)
	}

	// Issue an FT on the seeded series — it must succeed with a non-empty Number.
	input := sampleFTInput()
	input.SeriesID = "FT2099"
	input.Idem = app.IdempotencyKey{Key: "ft-seed-001", Fingerprint: "fp-seed-001"}
	doc, err := svc.Invoicing.IssueInvoice(ctx, tenant, input)
	if err != nil {
		t.Fatalf("IssueInvoice on seeded series: %v", err)
	}
	if doc.Number == "" {
		t.Fatalf("expected non-empty Number, got empty")
	}

	// Seeding the same ID again must return KindConflict.
	err2 := svc.Series.SeedRegisteredSeries(ctx, tenant, "FT2099", app.DocFT, "ATCODESEED1", "2026-04-01")
	if err2 == nil {
		t.Fatal("expected conflict error on duplicate seed, got nil")
	}
	if app.KindOf(err2) != app.KindConflict {
		t.Fatalf("expected KindConflict, got %v", app.KindOf(err2))
	}
}
