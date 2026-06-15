package main

import (
	"context"
	"fmt"
	"log"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/certkit"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// appIssuer satisfies certkit.Issuer by routing every family through the
// internal/app service layer (InvoicingService), so the certification scenarios
// run end-to-end against the application services instead of the domain layer
// directly. Series IDs follow the fixtures convention "<TYPE>2026".
//
// It ticks the shared clock once per issuance — the same minute-stepping the
// demo's DomainIssuer does — before delegating, so the service (which only reads
// Clock.Now()) stamps a distinct SystemEntryDate per document, matching cmd/demo.
type appIssuer struct {
	svc      *app.Services
	tenant   string
	sourceID string
	clock    *certkit.Clock
	ctx      context.Context
	n        int
}

func newAppIssuer(svc *app.Services, tenant, sourceID string, clock *certkit.Clock) *appIssuer {
	return &appIssuer{svc: svc, tenant: tenant, sourceID: sourceID, clock: clock, ctx: context.Background()}
}

// idem mints a fresh idempotency key per issuance. The smoke never retries, so
// key == fingerprint is enough; a real caller would fingerprint the payload.
func (a *appIssuer) idem() app.IdempotencyKey {
	a.n++
	k := fmt.Sprintf("appsmoke-%d", a.n)
	return app.IdempotencyKey{Key: k, Fingerprint: k}
}

func seriesID(dt domain.DocumentType) string { return string(dt) + "2026" }

func (a *appIssuer) Sales(draft *domain.DraftSalesInvoice, _ domain.IssueOptions) domain.SalesInvoice {
	a.clock.Tick()
	// The service looks the series up by ID from the repo and stamps its own
	// QR/options from the tenant (and the ND reader), so the caller's opts are
	// intentionally ignored.
	doc, err := a.svc.Invoicing.IssueSalesInvoice(a.ctx, a.tenant, *draft, seriesID(draft.DocumentType), a.sourceID, a.idem())
	if err != nil {
		log.Fatalf("app issue sales %s: %v", draft.DocumentType, err)
	}
	return doc
}

func (a *appIssuer) Work(draft *domain.DraftWorkDocument, _ domain.IssueOptions) domain.WorkDocument {
	a.clock.Tick()
	doc, err := a.svc.Invoicing.IssueWorkDocument(a.ctx, a.tenant, *draft, seriesID(draft.DocumentType), a.sourceID, a.idem())
	if err != nil {
		log.Fatalf("app issue work %s: %v", draft.DocumentType, err)
	}
	return doc
}

func (a *appIssuer) Stock(draft *domain.DraftStockMovement, _ domain.IssueOptions) domain.StockMovement {
	a.clock.Tick()
	doc, err := a.svc.Invoicing.IssueStockMovement(a.ctx, a.tenant, *draft, seriesID(draft.DocumentType), a.sourceID, a.idem())
	if err != nil {
		log.Fatalf("app issue stock %s: %v", draft.DocumentType, err)
	}
	return doc
}

func (a *appIssuer) Payment(draft *domain.PaymentDraft, totals domain.PaymentTotals, _ domain.IssueOptions) domain.Payment {
	a.clock.Tick()
	doc, err := a.svc.Invoicing.IssuePayment(a.ctx, a.tenant, *draft, seriesID(draft.Type), a.idem(), totals)
	if err != nil {
		log.Fatalf("app issue payment %s: %v", draft.Type, err)
	}
	return doc
}

func (a *appIssuer) Cancel(doc domain.SalesInvoice, reason string) domain.SalesInvoice {
	a.clock.Tick()
	out, err := a.svc.Invoicing.CancelDocument(a.ctx, a.tenant, doc.Number, reason)
	if err != nil {
		log.Fatalf("app cancel %s: %v", doc.Number.Format(), err)
	}
	return out
}
