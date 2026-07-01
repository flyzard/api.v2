package app_test

import (
	"context"
	"testing"
)

func TestPreviewTotals(t *testing.T) {
	svc, tenant := newTestServices(t)
	ctx := context.Background()
	in := sampleFTInput()

	preview, err := svc.Invoicing.PreviewTotals(ctx, tenant, in)
	if err != nil {
		t.Fatalf("PreviewTotals: %v", err)
	}

	// Preview must not consume the idempotency key or advance the series.
	// Issuing the same input right after must still succeed.
	issued, err := svc.Invoicing.IssueInvoice(ctx, tenant, in)
	if err != nil {
		t.Fatalf("IssueInvoice after PreviewTotals: %v", err)
	}

	if preview.GrossCents != issued.GrossCents {
		t.Errorf("GrossCents: preview=%d issued=%d", preview.GrossCents, issued.GrossCents)
	}
	if preview.NetCents != issued.NetCents {
		t.Errorf("NetCents: preview=%d issued=%d", preview.NetCents, issued.NetCents)
	}
	if len(preview.Breakdown) != len(issued.Breakdown) {
		t.Errorf("Breakdown bucket count: preview=%d issued=%d", len(preview.Breakdown), len(issued.Breakdown))
	}
}
