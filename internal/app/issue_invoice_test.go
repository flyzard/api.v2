package app_test

import (
	"context"
	"testing"
)

func TestIssueInvoiceFromValueInput(t *testing.T) {
	svc, tenant := newTestServices(t) // AllowUnknownAllocSource is false (default)
	ctx := context.Background()
	ft, err := svc.Invoicing.IssueInvoice(ctx, tenant, sampleFTInput())
	if err != nil {
		t.Fatalf("IssueInvoice FT: %v", err)
	}
	if ft.GrossCents == 0 || ft.Number == "" {
		t.Fatalf("empty view: %+v", ft)
	}
	nc, err := svc.Invoicing.IssueInvoice(ctx, tenant, sampleNCInput(ft.Number)) // references ft via a line DocRef
	if err != nil {
		t.Fatalf("IssueInvoice NC (allocation must match on identity, not the escape hatch): %v", err)
	}
	_ = nc
}
