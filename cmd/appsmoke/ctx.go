package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Ctx bundles the cross-cutting machinery every scenario needs and issues each
// family through the internal/app service layer (InvoicingService), recording the
// returned value into its projection Store. Fields are unexported so scenario
// bodies read verbatim; main constructs it via NewCtx.
//
// Every issue helper ticks the shared clock once before delegating, so the
// service (which only reads Clock.Now()) stamps a distinct SystemEntryDate per
// document. Series IDs follow the fixtures convention "<TYPE>2026".
type Ctx struct {
	f     *Fixtures
	clock *Clock
	store *Store

	svc      *app.Services
	tenant   string
	sourceID string
	ctx      context.Context
	n        int
}

// NewCtx wires a scenario context bound to one tenant. clock must be the same
// *Clock the service reads so timestamps stay monotonic across a run.
func NewCtx(f *Fixtures, clock *Clock, store *Store, svc *app.Services, tenant, sourceID string) *Ctx {
	return &Ctx{f: f, clock: clock, store: store, svc: svc, tenant: tenant, sourceID: sourceID, ctx: context.Background()}
}

// idem mints a fresh idempotency key per issuance. The smoke never retries, so
// key == fingerprint is enough; a real caller would fingerprint the payload.
func (c *Ctx) idem() app.IdempotencyKey {
	c.n++
	k := fmt.Sprintf("appsmoke-%d", c.n)
	return app.IdempotencyKey{Key: k, Fingerprint: k}
}

func seriesID(dt domain.DocumentType) string { return string(dt) + "2026" }

func (c *Ctx) atDay(date time.Time) {
	c.clock.SetBase(time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, date.Location()))
}

func (c *Ctx) line(code string, qty float64, when time.Time) domain.DocumentLine {
	it := c.f.Cat[code]
	return newLine(it.p, qty, it.price, it.tax(), when)
}

func (c *Ctx) nsLine(code string, qty float64, reason domain.Exemption, text string, when time.Time) domain.DocumentLine {
	it := c.f.Cat[code]
	return newLine(it.p, qty, it.price, nsTax(reason, text), when)
}

// ── issue + record helpers (scenarios cross into the app layer only here) ─────
//
// The caller's IssueOptions are intentionally ignored: the service looks the
// series up by ID and stamps its own QR/options from the tenant (and the ND
// reader), so a scenario's opts have no effect on the app path.

func (c *Ctx) issueSales(draft *domain.DraftSalesInvoice, _ domain.IssueOptions) domain.SalesInvoice {
	c.clock.Tick()
	doc, err := c.svc.Invoicing.IssueSalesInvoice(c.ctx, c.tenant, app.IssueSalesInvoiceRequest{
		Draft: *draft, SeriesID: seriesID(draft.DocumentType), SourceID: c.sourceID, Idem: c.idem(),
	})
	if err != nil {
		log.Fatalf("app issue sales %s: %v", draft.DocumentType, err)
	}
	c.store.recordSales(doc)
	return doc
}

func (c *Ctx) issueStock(draft *domain.DraftStockMovement, _ domain.IssueOptions) domain.StockMovement {
	c.clock.Tick()
	doc, err := c.svc.Invoicing.IssueStockMovement(c.ctx, c.tenant, app.IssueStockMovementRequest{
		Draft: *draft, SeriesID: seriesID(draft.DocumentType), SourceID: c.sourceID, Idem: c.idem(),
	})
	if err != nil {
		log.Fatalf("app issue stock %s: %v", draft.DocumentType, err)
	}
	c.store.recordStock(doc)
	return doc
}

func (c *Ctx) issueWork(draft *domain.DraftWorkDocument, _ domain.IssueOptions) domain.WorkDocument {
	c.clock.Tick()
	doc, err := c.svc.Invoicing.IssueWorkDocument(c.ctx, c.tenant, app.IssueWorkDocumentRequest{
		Draft: *draft, SeriesID: seriesID(draft.DocumentType), SourceID: c.sourceID, Idem: c.idem(),
	})
	if err != nil {
		log.Fatalf("app issue work %s: %v", draft.DocumentType, err)
	}
	c.store.recordWork(doc)
	return doc
}

func (c *Ctx) issuePayment(draft *domain.PaymentDraft, totals domain.PaymentTotals, _ domain.IssueOptions) domain.Payment {
	c.clock.Tick()
	doc, err := c.svc.Invoicing.IssuePayment(c.ctx, c.tenant, app.IssuePaymentRequest{
		Draft: *draft, SeriesID: seriesID(draft.Type), Idem: c.idem(), Totals: totals,
	})
	if err != nil {
		log.Fatalf("app issue payment %s: %v", draft.Type, err)
	}
	c.store.recordPayment(doc)
	return doc
}

func (c *Ctx) cancel(doc domain.SalesInvoice, reason string) domain.SalesInvoice {
	c.clock.Tick()
	out, err := c.svc.Invoicing.CancelDocument(c.ctx, c.tenant, doc.Number, reason)
	if err != nil {
		log.Fatalf("app cancel %s: %v", doc.Number.Format(), err)
	}
	c.store.recordSales(out)
	return out
}
