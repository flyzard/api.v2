package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
)

// Ctx bundles the cross-cutting machinery every scenario needs and issues each
// family through the internal/app value-in service layer (InvoicingService),
// recording the returned IssuedView into its projection Store. Fields are
// unexported so scenario bodies read verbatim; main constructs it via NewCtx.
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

func seriesID(dt string) string { return dt + "2026" }

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func (c *Ctx) atDay(date time.Time) {
	c.clock.SetBase(time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, date.Location()))
}

// line builds a value-in LineInput from a catalogue code at the catalogue price
// and default tax.
func (c *Ctx) line(code string, qty float64, when time.Time) app.LineInput {
	it := c.f.Cat[code]
	return newLine(it, qty, it.priceCents, it.tax(), when)
}

// nsLine overrides a catalogue line's tax with a "não sujeito" descriptor.
func (c *Ctx) nsLine(code string, qty float64, reason, text string, when time.Time) app.LineInput {
	it := c.f.Cat[code]
	return newLine(it, qty, it.priceCents, nsTax(reason, text), when)
}

// ── issue + record helpers (scenarios cross into the app layer only here) ─────

func (c *Ctx) issueSales(in app.IssueInvoiceInput) app.IssuedView {
	c.clock.Tick()
	in.SeriesID = seriesID(in.DocType)
	in.SourceID = c.sourceID
	in.IssuedBy = c.f.IssuerUser
	in.Idem = c.idem()
	doc, err := c.svc.Invoicing.IssueInvoice(c.ctx, c.tenant, in)
	if err != nil {
		log.Fatalf("app issue sales %s: %v", in.DocType, err)
	}
	c.store.record(doc)
	return doc
}

func (c *Ctx) issueStock(in app.IssueStockInput) app.IssuedView {
	c.clock.Tick()
	in.SeriesID = seriesID(in.DocType)
	in.SourceID = c.sourceID
	in.IssuedBy = c.f.IssuerUser
	in.Idem = c.idem()
	doc, err := c.svc.Invoicing.IssueStockMovement(c.ctx, c.tenant, in)
	if err != nil {
		log.Fatalf("app issue stock %s: %v", in.DocType, err)
	}
	c.store.record(doc)
	return doc
}

func (c *Ctx) issueWork(in app.IssueWorkInput) app.IssuedView {
	c.clock.Tick()
	in.SeriesID = seriesID(in.DocType)
	in.SourceID = c.sourceID
	in.IssuedBy = c.f.IssuerUser
	in.Idem = c.idem()
	doc, err := c.svc.Invoicing.IssueWorkDocument(c.ctx, c.tenant, in)
	if err != nil {
		log.Fatalf("app issue work %s: %v", in.DocType, err)
	}
	c.store.record(doc)
	return doc
}

func (c *Ctx) issuePayment(in app.IssuePaymentInput) app.IssuedView {
	c.clock.Tick()
	in.SeriesID = seriesID(in.Type)
	in.Idem = c.idem()
	doc, err := c.svc.Invoicing.IssuePayment(c.ctx, c.tenant, in)
	if err != nil {
		log.Fatalf("app issue payment %s: %v", in.Type, err)
	}
	c.store.record(doc)
	return doc
}

func (c *Ctx) cancel(number, reason string) app.IssuedView {
	c.clock.Tick()
	out, err := c.svc.Invoicing.CancelDocument(c.ctx, c.tenant, number, reason)
	if err != nil {
		log.Fatalf("app cancel %s: %v", number, err)
	}
	c.store.record(out)
	return out
}

// ─── tax + line builders ────────────────────────────────────────────────────

func taxRED() *app.LineTaxInput {
	return &app.LineTaxInput{Kind: "VAT", Region: app.RegionPT, Category: app.RateReduced}
}

func taxINT() *app.LineTaxInput {
	return &app.LineTaxInput{Kind: "VAT", Region: app.RegionPT, Category: app.RateIntermediate}
}

func taxNOR() *app.LineTaxInput {
	return &app.LineTaxInput{Kind: "VAT", Region: app.RegionPT, Category: app.RateNormal}
}

func taxEXEMPT(code, reason string) *app.LineTaxInput {
	return &app.LineTaxInput{Kind: "VAT", Region: app.RegionPT, Category: app.RateExempt, ExemptionCode: code, ExemptionReason: reason}
}

// nsTax builds a "não sujeito" (NS) line tax — valued line, zero VAT — for the
// own-asset / consignment / return movements (GA/GC/GD).
func nsTax(reason, text string) *app.LineTaxInput {
	return &app.LineTaxInput{Kind: "NS", Region: app.RegionPT, ExemptionCode: reason, ExemptionReason: text}
}

// newLine wires the product snapshot + price + TaxPointDate so each scenario only
// declares what varies. The line description derives from the catalogue per F-SAFT-9.
func newLine(it catItem, qty float64, priceCents int64, tax *app.LineTaxInput, when time.Time) app.LineInput {
	return app.LineInput{
		ProductCode:        it.code,
		ProductType:        it.ptype,
		ProductDescription: it.desc,
		ProductNumberCode:  it.code,
		Unit:               it.unit,
		Quantity:           qty,
		UnitPriceCents:     priceCents,
		TaxPointDate:       ymd(when),
		Tax:                tax,
	}
}

// ─── draft builders ─────────────────────────────────────────────────────────

func (c *Ctx) salesInput(dt string, cust app.CustomerInput, date time.Time, lines ...app.LineInput) app.IssueInvoiceInput {
	return app.IssueInvoiceInput{DocType: dt, Customer: cust, Date: ymd(date), Lines: lines}
}

func (c *Ctx) workInput(dt string, cust app.CustomerInput, date time.Time, lines ...app.LineInput) app.IssueWorkInput {
	return app.IssueWorkInput{DocType: dt, Customer: cust, Date: ymd(date), Lines: lines}
}

func (c *Ctx) stockInput(dt string, cust app.CustomerInput, date time.Time, from, to *app.AddressInput, start time.Time, lines ...app.LineInput) app.IssueStockInput {
	return app.IssueStockInput{
		DocType: dt, Customer: cust, Date: ymd(date), Lines: lines,
		ShipFrom: from, ShipTo: to, MovementStartTime: start.Format(time.RFC3339),
	}
}

func ship(detail, city, zip string) *app.AddressInput {
	return &app.AddressInput{Detail: detail, City: city, PostalCode: zip}
}

func days(n int) *int { return &n }
