package main

import (
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// ctx bundles the cross-cutting machinery every scenario needs.
type ctx struct {
	f      *fixtures
	signer domain.Signer
	clock  *monotonicClock
	store  *memoryStore
	qr     domain.QRConfig
}

// ─── tax + line builders ────────────────────────────────────────────────────

func taxRED() domain.LineTax {
	return must(domain.NewVATLineTax(domain.PT, domain.TaxReduced, "", ""))
}

func taxINT() domain.LineTax {
	return must(domain.NewVATLineTax(domain.PT, domain.TaxIntermediate, "", ""))
}

func taxNOR() domain.LineTax {
	return must(domain.NewVATLineTax(domain.PT, domain.TaxNormal, "", ""))
}

func taxEXEMPT(code domain.Exemption, reason string) domain.LineTax {
	return must(domain.NewVATLineTax(domain.PT, domain.TaxExempt, code, reason))
}

// newLine wires the boilerplate of Product snapshot + TaxPointDate so each
// scenario only declares what varies. The line description is derived from
// the product per F-SAFT-9; AddLine assigns LineNumber on insertion.
func newLine(p domain.Product, qty float64, unit float64, tax domain.LineTax, when time.Time) domain.DocumentLine {
	return domain.DocumentLine{
		Product:      p,
		Quantity:     must(domain.NewQuantity(qty)),
		UnitPrice:    must(domain.NewMoney(unit)),
		TaxPointDate: when,
		Tax:          tax,
	}
}

// addLines populates a draft via AddLine so LineNumber auto-sequences.
func addLines(d *domain.CommonDraftDocument, lines ...domain.DocumentLine) {
	for _, l := range lines {
		d.AddLine(l)
	}
}

// ─── issue + record helpers ─────────────────────────────────────────────────

func (c *ctx) issueSales(draft *domain.DraftSalesInvoice, opts domain.IssueOptions) domain.SalesInvoice {
	doc, err := domain.IssueSalesInvoice(draft, c.f.Series[draft.DocumentType], c.signer, c.f.IssuerUser.Email, c.clock.Tick(), opts, c.qr)
	if err != nil {
		log.Fatalf("issue sales %s: %v", draft.DocumentType, err)
	}
	c.store.recordSales(doc)
	return doc
}

func (c *ctx) issueStock(draft *domain.DraftStockMovement, opts domain.IssueOptions) domain.StockMovement {
	doc, err := domain.IssueStockMovement(draft, c.f.Series[draft.DocumentType], c.signer, c.f.IssuerUser.Email, c.clock.Tick(), opts, c.qr)
	if err != nil {
		log.Fatalf("issue stock %s: %v", draft.DocumentType, err)
	}
	c.store.recordStock(doc)
	return doc
}

func (c *ctx) issueWork(draft *domain.DraftWorkDocument, opts domain.IssueOptions) domain.WorkDocument {
	doc, err := domain.IssueWorkDocument(draft, c.f.Series[draft.DocumentType], c.signer, c.f.IssuerUser.Email, c.clock.Tick(), opts, c.qr)
	if err != nil {
		log.Fatalf("issue work %s: %v", draft.DocumentType, err)
	}
	c.store.recordWork(doc)
	return doc
}

func (c *ctx) issuePayment(draft *domain.PaymentDraft, totals domain.PaymentTotals, opts domain.IssueOptions) domain.Payment {
	doc, err := domain.IssuePayment(draft, c.f.Series[draft.Type], c.clock.Tick(), totals, opts)
	if err != nil {
		log.Fatalf("issue payment %s: %v", draft.Type, err)
	}
	c.store.recordPayment(doc)
	return doc
}

func (c *ctx) common(dt domain.DocumentType, cust domain.Customer, date time.Time, lines []domain.DocumentLine) domain.CommonDraftDocument {
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: dt,
			Customer:     cust,
			Date:         date,
			IssuedBy:     c.f.IssuerUser,
		},
		Series: *c.f.Series[dt],
	}
	addLines(&cd, lines...)
	return cd
}

func (c *ctx) salesDraft(dt domain.DocumentType, cust domain.Customer, date time.Time, fields domain.SalesInvoiceFields, lines ...domain.DocumentLine) *domain.DraftSalesInvoice {
	return &domain.DraftSalesInvoice{
		CommonDraftDocument: c.common(dt, cust, date, lines),
		SalesInvoiceFields:  fields,
	}
}

func (c *ctx) workDraft(dt domain.DocumentType, cust domain.Customer, date time.Time, lines ...domain.DocumentLine) *domain.DraftWorkDocument {
	return &domain.DraftWorkDocument{CommonDraftDocument: c.common(dt, cust, date, lines)}
}

func (c *ctx) stockDraft(dt domain.DocumentType, cust domain.Customer, date time.Time, from, to *domain.ShippingPoint, start time.Time, lines ...domain.DocumentLine) *domain.DraftStockMovement {
	return &domain.DraftStockMovement{
		CommonDraftDocument: c.common(dt, cust, date, lines),
		StockMovementFields: domain.StockMovementFields{
			MovementStartTime: start,
			ShipFrom:          from,
			ShipTo:            to,
		},
	}
}

func mustShip(detail, city, zip string) *domain.ShippingPoint {
	addr := must(domain.NewAddress(detail, city, zip, "PT"))
	return &domain.ShippingPoint{Address: &addr}
}

// ─── 5.1 ────────────────────────────────────────────────────────────────────

func scenario51(c *ctx, today time.Time) {
	banner("5.1", "Fatura simplificada (art. 40.º CIVA) — cliente com NIF")
	draft := c.salesDraft(domain.FS, c.f.CustWithNIF, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-NOR"], 1, 50.00, taxNOR(), today),
	)
	doc := c.issueSales(draft, domain.IssueOptions{IssuerEAC: c.f.Issuer.EACCode})
	printJSON("FS issued", doc)
	salesSummary("5.1", doc)
}

// ─── 5.2 ────────────────────────────────────────────────────────────────────

func scenario52(c *ctx, today time.Time) {
	banner("5.2", "Fatura (art. 36.º CIVA) anulada — PDF + SAF-T DocumentStatus")
	draft := c.salesDraft(domain.FT, c.f.CustWithNIF, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-NOR"], 1, 100.00, taxNOR(), today),
	)
	due := today.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued (pre-cancellation)", doc)

	cancelAt := c.clock.Tick()
	if err := doc.Cancel("Erro de emissão", cancelAt); err != nil {
		log.Fatalf("cancel: %v", err)
	}
	c.store.recordSales(doc)
	printJSON("FT after Cancel (DB state)", doc)

	printCancelledPDF(doc)
	printSAFTCancelRow(doc)
	salesSummary("5.2", doc)
}

// ─── 5.3 ────────────────────────────────────────────────────────────────────
//
// "Documento suscetível de ser entregue ao cliente para conferência de
// transmissão de bens ou de prestação de serviços" — a working-document that
// the customer receives before billing. NE (nota de encomenda) fits the bill
// and conveniently feeds 5.4 via OrderReferences.

var (
	doc53 domain.WorkDocument
	doc54 domain.SalesInvoice
	doc56 domain.SalesInvoice
)

func scenario53(c *ctx, today time.Time) {
	banner("5.3", "Working document para conferência (NE — nota de encomenda)")
	draft := c.workDraft(domain.NE, c.f.CustWithNIF, today,
		newLine(c.f.Products["P-NOR"], 2, 30.00, taxNOR(), today),
	)
	doc53 = c.issueWork(draft, domain.IssueOptions{})
	printJSON("NE issued", doc53)
	workSummary("5.3", doc53)
}

// ─── 5.4 ────────────────────────────────────────────────────────────────────

func scenario54(c *ctx, today time.Time) {
	banner("5.4", "Fatura baseada no documento 5.3 — gera OrderReferences")
	line := newLine(c.f.Products["P-NOR"], 2, 30.00, taxNOR(), today)
	line.OrderReferences = []domain.OrderReference{{
		OriginatingON: doc53.Number.Format(),
		OrderDate:     &doc53.Date,
	}}

	draft := c.salesDraft(domain.FT, c.f.CustWithNIF, today, domain.SalesInvoiceFields{}, line)
	due := today.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc54 = c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued with OrderReferences → NE", doc54)

	if err := doc53.MarkBilled(doc54.Number, c.clock.Tick()); err != nil {
		log.Fatalf("mark NE billed: %v", err)
	}
	c.store.recordWork(doc53)
	printJSON("NE after MarkBilled (Status=F)", doc53)
	salesSummary("5.4", doc54)
}

// ─── 5.5 ────────────────────────────────────────────────────────────────────

func scenario55(c *ctx, today time.Time) {
	banner("5.5", "Nota de crédito sobre a fatura 5.4 — gera References")
	line := newLine(c.f.Products["P-NOR"], 1, 30.00, taxNOR(), today)
	line.References = []domain.DocReference{{
		Reference: doc54.Number.Format(),
		Reason:    "Devolução parcial — produto danificado",
	}}

	draft := c.salesDraft(domain.NC, c.f.CustWithNIF, today, domain.SalesInvoiceFields{}, line)
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("NC issued with References → FT", doc)
	salesSummary("5.5", doc)
}

// ─── 5.6 ────────────────────────────────────────────────────────────────────

func scenario56(c *ctx, today time.Time) {
	banner("5.6", "Fatura com 4 linhas — Reduzida / Isento (M07) / Intermédia / Normal")
	draft := c.salesDraft(domain.FT, c.f.CustWithNIF, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-RED"], 2, 1.50, taxRED(), today),
		newLine(c.f.Products["P-EXEMPT"], 1, 20.00, taxEXEMPT(domain.M07, "Isento artigo 9.º CIVA"), today),
		newLine(c.f.Products["P-INT"], 1, 5.00, taxINT(), today),
		newLine(c.f.Products["P-NOR"], 1, 10.00, taxNOR(), today),
	)
	due := today.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc56 = c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT 4 linhas (RED/ISE/INT/NOR)", doc56)
	salesSummary("5.6", doc56)
}

// ─── 5.7 ────────────────────────────────────────────────────────────────────

func scenario57(c *ctx, today time.Time) {
	banner("5.7", "Fatura com descontos por linha (Percent + Amount) — AT-compliant")
	// AT cert §5.7 (round 3348): line discounts must surface as per-line
	// SettlementAmount, never as a doc-level Settlement. Exercising both
	// PercentDiscount (line1) and AmountDiscount (line2) to cover both paths.
	line1 := newLine(c.f.Products["P-NOR"], 100, 0.55, taxNOR(), today)
	line1.Discount = must(domain.NewPercentDiscount(8.8))
	line2 := newLine(c.f.Products["P-SERVICE"], 1, 10.00, taxNOR(), today)
	line2.Discount = must(domain.NewAmountDiscount(must(domain.NewMoney(5.00))))

	draft := c.salesDraft(domain.FT, c.f.CustWithNIF, today, domain.SalesInvoiceFields{}, line1, line2)
	due := today.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued (line discounts + PaymentTerms)", doc)
	salesSummary("5.7", doc)
}

// ─── 5.8 ────────────────────────────────────────────────────────────────────

func scenario58(c *ctx, today time.Time) {
	banner("5.8", "Fatura em moeda estrangeira (USD)")
	draft := c.salesDraft(domain.FT, c.f.CustForeign, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-SERVICE"], 4, 80.00, taxNOR(), today),
	)
	draft.CalculateTotals()
	currency := must(domain.NewCurrency(
		must(domain.NewCurrencyCode("USD")),
		draft.Totals.GrossTotal,
		must(domain.NewExchangeRate(1.085000)),
		today,
	))
	draft.Currency = &currency
	due := today.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued (EUR totals + Currency block)", doc)
	salesSummary("5.8", doc)
}

// ─── 5.9 ────────────────────────────────────────────────────────────────────

func scenario59(c *ctx, today time.Time) {
	banner("5.9", "Cliente identificado sem NIF — GrossTotal < €1,00 — SystemEntryDate < 10h")
	// Clock started at 09:00 and only ticks minutes per issue, so SystemEntryDate
	// stays comfortably before 10:00 by the time this scenario runs.
	draft := c.salesDraft(domain.FS, c.f.CustNoNIF1, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-RED"], 1, 0.50, taxNOR(), today),
	)
	doc := c.issueSales(draft, domain.IssueOptions{IssuerEAC: c.f.Issuer.EACCode})
	printJSON("FS issued — pequeno valor, manhã", doc)
	salesSummary("5.9", doc)
}

// ─── 5.10 ───────────────────────────────────────────────────────────────────

func scenario510(c *ctx, today time.Time) {
	banner("5.10", "Outro cliente identificado sem NIF")
	draft := c.salesDraft(domain.FS, c.f.CustNoNIF2, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-INT"], 2, 30.00, taxNOR(), today),
	)
	doc := c.issueSales(draft, domain.IssueOptions{IssuerEAC: c.f.Issuer.EACCode})
	printJSON("FS issued — outro cliente sem NIF", doc)
	salesSummary("5.10", doc)
}

// ─── 5.11 ───────────────────────────────────────────────────────────────────

func scenario511(c *ctx, today time.Time) {
	banner("5.11", "Duas guias de remessa — uma valorizada, outra não")

	from := mustShip("Armazém Central, Rua Industrial 5", "Lisboa", "1900-001")
	to := mustShip("Loja, Av. dos Aliados 100", "Porto", "4000-100")
	startTime := c.clock.Tick().Add(time.Hour) // movement starts after system entry

	v := c.issueStock(c.stockDraft(domain.GR, c.f.CustWithNIF, today, from, to, startTime,
		newLine(c.f.Products["P-CRATE"], 5, 12.00, taxNOR(), today),
	), domain.IssueOptions{})
	printJSON("GR valorizada", v)
	stockSummary("5.11a (valorizada)", v)

	// Non-valued GR — UnitPrice=0 with Tax=nil yields gross 0 (regras §3.6 / I-H7).
	nv := c.issueStock(c.stockDraft(domain.GR, c.f.CustWithNIF, today, from, to, startTime.Add(30*time.Minute),
		newLine(c.f.Products["P-CRATE"], 3, 0, nil, today),
	), domain.IssueOptions{})
	printJSON("GR não valorizada (UnitPrice=0, Tax=nil)", nv)
	stockSummary("5.11b (não valorizada)", nv)
}

// ─── 5.12 ───────────────────────────────────────────────────────────────────

func scenario512(c *ctx, today time.Time) {
	banner("5.12", "Orçamento (OR)")
	draft := c.workDraft(domain.OR, c.f.CustWithNIF, today,
		newLine(c.f.Products["P-SERVICE"], 8, 75.00, taxNOR(), today),
	)
	doc := c.issueWork(draft, domain.IssueOptions{})
	printJSON("OR issued", doc)
	workSummary("5.12", doc)
}

// ─── 5.13 ───────────────────────────────────────────────────────────────────

func scenario513(c *ctx, today time.Time) {
	banner("5.13", "Um exemplo de cada um dos restantes tipos de documento")

	c.scenario513FR(today)
	c.scenario513ND(today)
	c.scenario513Transport(domain.GT, "Guia de transporte", today)
	c.scenario513Transport(domain.GA, "Guia de movimentação de ativos próprios", today)
	c.scenario513Transport(domain.GC, "Guia de consignação", today)
	c.scenario513Transport(domain.GD, "Guia de devolução", today)
	c.scenario513Working(domain.PF, "Fatura pró-forma", today)
	c.scenario513Working(domain.CM, "Consulta de mesa", today)
	c.scenario513Working(domain.FC, "Documento de consignação", today)
	c.scenario513Working(domain.FO, "Folha de obra", today)
	c.scenario513RC(today)
	c.scenario513RG(today)
}

func (c *ctx) scenario513FR(today time.Time) {
	fmt.Println("\n--- 5.13a · Fatura-recibo (FR) ---")
	draft := c.salesDraft(domain.FR, c.f.CustWithNIF, today, domain.SalesInvoiceFields{},
		newLine(c.f.Products["P-NOR"], 1, 25.00, taxNOR(), today),
	)
	draft.CalculateTotals()
	draft.Payments = []domain.FRPayment{{
		Mechanism: domain.PaymentMechanismCash,
		Amount:    draft.Totals.GrossTotal,
		Date:      today,
	}}
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FR issued", doc)
	salesSummary("5.13a FR", doc)
}

func (c *ctx) scenario513ND(today time.Time) {
	fmt.Println("\n--- 5.13b · Nota de débito (ND) sobre 5.6 ---")
	// ND adjusts values, not quantities — match the originating P-NOR line's qty exactly.
	ndLine := newLine(c.f.Products["P-NOR"], 1, 2.00, taxNOR(), today)
	ndLine.Quantity = doc56.Lines[3].Quantity
	ndLine.References = []domain.DocReference{{
		Reference: doc56.Number.Format(),
		Reason:    "Acerto de preço",
	}}
	draft := c.salesDraft(domain.ND, c.f.CustWithNIF, today, domain.SalesInvoiceFields{}, ndLine)
	doc := c.issueSales(draft, domain.IssueOptions{Reader: c.store})
	printJSON("ND issued (references FT 5.6, same product+qty)", doc)
	salesSummary("5.13b ND", doc)
}

func (c *ctx) scenario513Transport(dt domain.DocumentType, title string, today time.Time) {
	fmt.Printf("\n--- 5.13 · %s (%s) ---\n", title, dt)
	from := mustShip("Polo Logístico Sul", "Setúbal", "2900-100")
	to := mustShip("Av. Norte 1", "Braga", "4700-100")
	draft := c.stockDraft(dt, c.f.CustWithNIF, today, from, to, c.clock.Now().Add(2*time.Hour),
		newLine(c.f.Products["P-CRATE"], 4, 15.00, taxNOR(), today),
	)
	doc := c.issueStock(draft, domain.IssueOptions{})
	printJSON(string(dt)+" issued", doc)
	stockSummary("5.13 "+string(dt), doc)
}

func (c *ctx) scenario513Working(dt domain.DocumentType, title string, today time.Time) {
	fmt.Printf("\n--- 5.13 · %s (%s) ---\n", title, dt)
	draft := c.workDraft(dt, c.f.CustWithNIF, today,
		newLine(c.f.Products["P-SERVICE"], 1, 40.00, taxNOR(), today),
	)
	doc := c.issueWork(draft, domain.IssueOptions{})
	printJSON(string(dt)+" issued", doc)
	workSummary("5.13 "+string(dt), doc)
}

func (c *ctx) scenario513RC(today time.Time) {
	fmt.Println("\n--- 5.13 · Recibo (RC) liquida a fatura 5.6 ---")
	line := domain.PaymentLine{
		LineNumber: 1,
		SourceDocuments: []domain.SourceDocumentID{{
			OriginatingON: doc56.Number.Format(),
			InvoiceDate:   doc56.Date,
			Description:   "Liquidação integral",
		}},
		Movement: domain.CreditAmount{Value: doc56.Totals.GrossTotal},
		Tax:      taxNOR(),
	}
	draft := &domain.PaymentDraft{
		Type:            domain.RC,
		TransactionDate: today,
		Customer:        c.f.CustWithNIF,
		SourceID:        c.f.IssuerUser.Email,
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismBankTransfer,
			Amount:    doc56.Totals.GrossTotal,
			Date:      today,
		}},
		Lines: []domain.PaymentLine{line},
	}
	totals := domain.PaymentTotals{
		NetTotal:   doc56.Totals.NetTotal,
		TaxPayable: doc56.Totals.TaxTotal,
		GrossTotal: doc56.Totals.GrossTotal,
	}
	doc := c.issuePayment(draft, totals, domain.IssueOptions{})
	printJSON("RC issued", doc)
	paymentSummary("5.13 RC", doc)
}

func (c *ctx) scenario513RG(today time.Time) {
	fmt.Println("\n--- 5.13 · Outro recibo (RG) — adiantamento sem fatura específica ---")
	advance := must(domain.NewMoney(50.00))
	line := domain.PaymentLine{
		LineNumber: 1,
		SourceDocuments: []domain.SourceDocumentID{{
			OriginatingON: "Adiantamento " + today.Format("2006-01-02"),
			InvoiceDate:   today,
			Description:   "Adiantamento por conta de serviços futuros",
		}},
		Movement: domain.CreditAmount{Value: advance},
	}
	draft := &domain.PaymentDraft{
		Type:            domain.RG,
		TransactionDate: today,
		Customer:        c.f.CustWithNIF,
		SourceID:        c.f.IssuerUser.Email,
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismCash,
			Amount:    advance,
			Date:      today,
		}},
		Lines: []domain.PaymentLine{line},
	}
	totals := domain.PaymentTotals{
		NetTotal:   advance,
		TaxPayable: 0,
		GrossTotal: advance,
	}
	doc := c.issuePayment(draft, totals, domain.IssueOptions{})
	printJSON("RG issued", doc)
	paymentSummary("5.13 RG", doc)
}
