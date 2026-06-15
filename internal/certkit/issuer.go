package certkit

import (
	"log"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Issuer is the seam between a scenario and however a document gets issued. The
// scenarios are identical across binaries; only the Issuer differs: cmd/demo
// wires a DomainIssuer (straight to the domain layer), cmd/appsmoke wires one
// backed by the internal/app service layer. An Issuer only issues; the Ctx
// records the returned value into its projection Store.
type Issuer interface {
	Sales(draft *domain.DraftSalesInvoice, opts domain.IssueOptions) domain.SalesInvoice
	Stock(draft *domain.DraftStockMovement, opts domain.IssueOptions) domain.StockMovement
	Work(draft *domain.DraftWorkDocument, opts domain.IssueOptions) domain.WorkDocument
	Payment(draft *domain.PaymentDraft, totals domain.PaymentTotals, opts domain.IssueOptions) domain.Payment
	Cancel(doc domain.SalesInvoice, reason string) domain.SalesInvoice
}

// Ctx bundles the cross-cutting machinery every scenario needs. Fields are
// unexported so scenario bodies read verbatim; binaries construct it via NewCtx.
type Ctx struct {
	f     *Fixtures
	clock *Clock
	store *Store
	iss   Issuer
}

// NewCtx wires a scenario context. clock must be the same *Clock the Issuer ticks
// (when it ticks one at all) so timestamps stay monotonic across a run.
func NewCtx(f *Fixtures, clock *Clock, store *Store, iss Issuer) *Ctx {
	return &Ctx{f: f, clock: clock, store: store, iss: iss}
}

// ── issue + record helpers (the only place the Issuer seam is crossed) ───────

func (c *Ctx) issueSales(draft *domain.DraftSalesInvoice, opts domain.IssueOptions) domain.SalesInvoice {
	doc := c.iss.Sales(draft, opts)
	c.store.recordSales(doc)
	return doc
}

func (c *Ctx) issueStock(draft *domain.DraftStockMovement, opts domain.IssueOptions) domain.StockMovement {
	doc := c.iss.Stock(draft, opts)
	c.store.recordStock(doc)
	return doc
}

func (c *Ctx) issueWork(draft *domain.DraftWorkDocument, opts domain.IssueOptions) domain.WorkDocument {
	doc := c.iss.Work(draft, opts)
	c.store.recordWork(doc)
	return doc
}

func (c *Ctx) issuePayment(draft *domain.PaymentDraft, totals domain.PaymentTotals, opts domain.IssueOptions) domain.Payment {
	doc := c.iss.Payment(draft, totals, opts)
	c.store.recordPayment(doc)
	return doc
}

func (c *Ctx) cancel(doc domain.SalesInvoice, reason string) domain.SalesInvoice {
	out := c.iss.Cancel(doc, reason)
	c.store.recordSales(out)
	return out
}

// ── DomainIssuer: issues straight through the domain layer (cmd/demo) ─────────

// DomainIssuer reproduces cmd/demo's original behaviour: each family issued by
// its domain constructor, the per-document SystemEntryDate taken from a shared
// minute-stepping clock.
type DomainIssuer struct {
	f      *Fixtures
	clock  *Clock
	signer domain.Signer
	qr     domain.QRConfig
}

func NewDomainIssuer(f *Fixtures, clock *Clock, signer domain.Signer, qr domain.QRConfig) *DomainIssuer {
	return &DomainIssuer{f: f, clock: clock, signer: signer, qr: qr}
}

func (d *DomainIssuer) Sales(draft *domain.DraftSalesInvoice, opts domain.IssueOptions) domain.SalesInvoice {
	doc, err := domain.IssueSalesInvoice(draft, d.f.Series[draft.DocumentType], d.signer, d.f.IssuerUser.Email, d.clock.Tick(), opts, d.qr)
	if err != nil {
		log.Fatalf("issue sales %s: %v", draft.DocumentType, err)
	}
	return doc
}

func (d *DomainIssuer) Stock(draft *domain.DraftStockMovement, opts domain.IssueOptions) domain.StockMovement {
	doc, err := domain.IssueStockMovement(draft, d.f.Series[draft.DocumentType], d.signer, d.f.IssuerUser.Email, d.clock.Tick(), opts, d.qr)
	if err != nil {
		log.Fatalf("issue stock %s: %v", draft.DocumentType, err)
	}
	return doc
}

func (d *DomainIssuer) Work(draft *domain.DraftWorkDocument, opts domain.IssueOptions) domain.WorkDocument {
	doc, err := domain.IssueWorkDocument(draft, d.f.Series[draft.DocumentType], d.signer, d.f.IssuerUser.Email, d.clock.Tick(), opts, d.qr)
	if err != nil {
		log.Fatalf("issue work %s: %v", draft.DocumentType, err)
	}
	return doc
}

func (d *DomainIssuer) Payment(draft *domain.PaymentDraft, totals domain.PaymentTotals, opts domain.IssueOptions) domain.Payment {
	doc, err := domain.IssuePayment(draft, d.f.Series[draft.Type], d.clock.Tick(), totals, opts)
	if err != nil {
		log.Fatalf("issue payment %s: %v", draft.Type, err)
	}
	return doc
}

func (d *DomainIssuer) Cancel(doc domain.SalesInvoice, reason string) domain.SalesInvoice {
	if err := doc.Cancel(reason, d.clock.Tick()); err != nil {
		log.Fatalf("cancel: %v", err)
	}
	return doc
}
