package main

import (
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

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

// ─── draft builders ─────────────────────────────────────────────────────────

func (c *Ctx) common(dt domain.DocumentType, cust domain.Customer, date time.Time, lines []domain.DocumentLine) domain.CommonDraftDocument {
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: dt,
			Customer:     cust,
			Date:         date,
			IssuedBy:     c.f.IssuerUser,
		},
		Series: *c.f.Series[dt],
	}
	// AddLine auto-sequences each line's LineNumber.
	for _, l := range lines {
		cd.AddLine(l)
	}
	return cd
}

func (c *Ctx) salesDraft(dt domain.DocumentType, cust domain.Customer, date time.Time, fields domain.SalesInvoiceFields, lines ...domain.DocumentLine) *domain.DraftSalesInvoice {
	return &domain.DraftSalesInvoice{
		CommonDraftDocument: c.common(dt, cust, date, lines),
		SalesInvoiceFields:  fields,
	}
}

func (c *Ctx) workDraft(dt domain.DocumentType, cust domain.Customer, date time.Time, lines ...domain.DocumentLine) *domain.DraftWorkDocument {
	return &domain.DraftWorkDocument{CommonDraftDocument: c.common(dt, cust, date, lines)}
}

func (c *Ctx) stockDraft(dt domain.DocumentType, cust domain.Customer, date time.Time, from, to *domain.ShippingPoint, start time.Time, lines ...domain.DocumentLine) *domain.DraftStockMovement {
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

func Scenario51(c *Ctx, date time.Time) {
	banner("5.1", "Fatura simplificada (art. 40.º CIVA) — cliente com NIF")
	c.atDay(date)
	draft := c.salesDraft(domain.FS, c.f.Cust["C001"], date, domain.SalesInvoiceFields{},
		c.line("P007", 2, date),
	)
	doc := c.issueSales(draft, domain.IssueOptions{IssuerEAC: c.f.Issuer.EACCode})
	printJSON("FS issued", doc)
	salesSummary("5.1", doc)
	expectSales("5.1", doc, 6.90, 1.59, 8.49)
}

// ─── 5.2 ────────────────────────────────────────────────────────────────────

func Scenario52(c *Ctx, date time.Time) {
	banner("5.2", "Fatura (art. 36.º CIVA) anulada — PDF + SAF-T DocumentStatus")
	c.atDay(date)
	draft := c.salesDraft(domain.FT, c.f.Cust["C002"], date, domain.SalesInvoiceFields{},
		c.line("P003", 12, date),
		c.line("P004", 6, date),
	)
	due := date.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued (pre-cancellation)", doc)
	expectSales("5.2", doc, 253.80, 47.69, 301.49)

	doc = c.cancel(doc, "Anulação a pedido")
	printJSON("FT after Cancel (DB state)", doc)

	printCancelledPDF(doc)
	printSAFTCancelRow(doc)
	salesSummary("5.2", doc)
}

// ─── 5.3 ────────────────────────────────────────────────────────────────────
//
// DC (documento de conferência) was discontinued; the reviewed dataset uses PF
// (pro-forma) as the document handed to the customer for conferência before
// billing. It feeds 5.4 via OrderReferences (PF2026/1).

var (
	doc53 domain.WorkDocument
	doc54 domain.SalesInvoice
	doc56 domain.SalesInvoice
)

func Scenario53(c *Ctx, date time.Time) {
	banner("5.3", "Pró-forma para conferência (PF) — origem do 5.4")
	c.atDay(date)
	draft := c.workDraft(domain.PF, c.f.Cust["C003"], date,
		c.line("P011", 4, date),
		c.line("P003", 6, date),
	)
	doc53 = c.issueWork(draft, domain.IssueOptions{})
	printJSON("PF issued", doc53)
	workSummary("5.3", doc53)
	expectDoc("5.3", doc53.Totals, 267.00, 34.71, 301.71)
}

// ─── 5.4 ────────────────────────────────────────────────────────────────────

func Scenario54(c *Ctx, date time.Time) {
	banner("5.4", "Fatura baseada na pró-forma 5.3 — gera OrderReferences")
	c.atDay(date)
	line1 := c.line("P011", 4, date)
	line1.OrderReferences = []domain.OrderReference{{
		OriginatingON: doc53.Number.Format(),
		OrderDate:     &doc53.Date,
	}}

	draft := c.salesDraft(domain.FT, c.f.Cust["C003"], date, domain.SalesInvoiceFields{},
		line1,
		c.line("P003", 6, date),
	)
	due := date.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc54 = c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued with OrderReferences → PF", doc54)
	expectSales("5.4", doc54, 267.00, 34.71, 301.71)

	if err := doc53.MarkBilled(doc54.Number, c.clock.Tick()); err != nil {
		log.Fatalf("mark PF billed: %v", err)
	}
	c.store.recordWork(doc53)
	printJSON("PF after MarkBilled (Status=F)", doc53)
	salesSummary("5.4", doc54)
}

// ─── 5.5 ────────────────────────────────────────────────────────────────────

func Scenario55(c *Ctx, date time.Time) {
	banner("5.5", "Nota de crédito sobre a fatura 5.4 — References ao nível da linha")
	c.atDay(date)
	line := c.line("P011", 1, date)
	line.References = []domain.DocReference{{
		Reference: doc54.Number.Format(),
		Reason:    "Devolução de 1 caixa de vinho",
	}}

	draft := c.salesDraft(domain.NC, c.f.Cust["C003"], date, domain.SalesInvoiceFields{}, line)
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("NC issued with line References → FT", doc)
	salesSummary("5.5", doc)
	expectSales("5.5", doc, 53.40, 6.94, 60.34)
}

// ─── 5.6 ────────────────────────────────────────────────────────────────────

func Scenario56(c *Ctx, date time.Time) {
	banner("5.6", "Fatura com 4 linhas — Reduzida / Isento (M07) / Intermédia / Normal")
	c.atDay(date)
	draft := c.salesDraft(domain.FT, c.f.Cust["C002"], date, domain.SalesInvoiceFields{},
		c.line("P001", 3, date),
		c.line("P002", 1, date), // isento M07 (catalogue default)
		c.line("P003", 6, date),
		c.line("P004", 2, date),
	)
	due := date.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc56 = c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT 4 linhas (RED/ISE/INT/NOR)", doc56)
	salesSummary("5.6", doc56)
	expectSales("5.6", doc56, 256.27, 18.44, 274.71)
}

// ─── 5.7 ────────────────────────────────────────────────────────────────────

func Scenario57(c *Ctx, date time.Time) {
	banner("5.7", "Fatura — desconto de linha 8.8% + desconto global (Settlement)")
	c.atDay(date)
	// L1: qty 100 × €0.55 with an 8.8% line discount (the UnitPrice 5dp reflects
	// it → 0.5016, net 50.16). L2: qty 10 × €1.80, net 18.00. Pre-global net 68.16.
	//
	// The reviewed dataset reads the global discount as pure pronto-pagamento
	// (Settlement only, base untouched → Net 68.16 / Tax 15.68 / Settlement 3.41).
	// This engine models the §5.7 global discount as COMMERCIAL: GlobalDiscount
	// prorates into the lines, so it reduces base and VAT and surfaces the same
	// amount as Settlement. Per the chosen interpretation we assert the engine's
	// numbers (Net 64.75 / Tax 14.89 / Gross 79.64 / Settlement 3.41).
	line1 := c.line("P005", 100, date)
	line1.Discount = must(domain.NewPercentDiscount(8.8))
	line2 := c.line("P006", 10, date)

	draft := c.salesDraft(domain.FT, c.f.Cust["C002"], date, domain.SalesInvoiceFields{}, line1, line2)
	draft.GlobalDiscount = must(domain.NewAmountDiscount(must(domain.NewMoney(3.41))))
	due := date.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued (line discount 8.8% + global discount €3.41)", doc)
	salesSummary("5.7", doc)
	expectSales("5.7", doc, 64.75, 14.89, 79.64)
}

// ─── 5.8 ────────────────────────────────────────────────────────────────────

func Scenario58(c *Ctx, date time.Time) {
	banner("5.8", "Fatura em moeda estrangeira (USD)")
	c.atDay(date)
	draft := c.salesDraft(domain.FT, c.f.Cust["C006"], date, domain.SalesInvoiceFields{},
		c.line("P004", 3, date),
	)
	draft.CalculateTotals()
	currency := must(domain.NewCurrency(
		must(domain.NewCurrencyCode("USD")),
		draft.Totals.GrossTotal,
		must(domain.NewExchangeRate(1.085000)),
		date,
	))
	draft.Currency = &currency
	due := date.AddDate(0, 0, 30)
	draft.PaymentTerms = &due
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FT issued (EUR totals + Currency block)", doc)
	salesSummary("5.8", doc)
	expectSales("5.8", doc, 73.50, 16.91, 90.41)
}

// ─── 5.9 ────────────────────────────────────────────────────────────────────

func Scenario59(c *Ctx, date time.Time) {
	banner("5.9", "Cliente identificado sem NIF — GrossTotal < €1,00 — SystemEntryDate < 10h")
	c.atDay(date) // 09:00 + a few ticks → entry stays before 10:00
	draft := c.salesDraft(domain.FS, c.f.Cust["C004"], date, domain.SalesInvoiceFields{},
		c.line("P008", 12, date),
	)
	doc := c.issueSales(draft, domain.IssueOptions{IssuerEAC: c.f.Issuer.EACCode})
	printJSON("FS issued — pequeno valor, manhã", doc)
	salesSummary("5.9", doc)
	expectSales("5.9", doc, 0.60, 0.14, 0.74)
}

// ─── 5.10 ───────────────────────────────────────────────────────────────────

func Scenario510(c *Ctx, date time.Time) {
	banner("5.10", "Outro cliente identificado sem NIF")
	c.atDay(date)
	draft := c.salesDraft(domain.FS, c.f.Cust["C005"], date, domain.SalesInvoiceFields{},
		c.line("P007", 3, date),
	)
	doc := c.issueSales(draft, domain.IssueOptions{IssuerEAC: c.f.Issuer.EACCode})
	printJSON("FS issued — outro cliente sem NIF", doc)
	salesSummary("5.10", doc)
	expectSales("5.10", doc, 10.35, 2.38, 12.73)
}

// ─── 5.11 ───────────────────────────────────────────────────────────────────

func Scenario511(c *Ctx, date time.Time) {
	banner("5.11", "Guia de transporte valorizada (GT) + guia de remessa não valorizada (GR)")
	c.atDay(date)

	from := mustShip("Armazém do emitente", "Benedita", "2475-123")
	to := mustShip("Mercearia Central, Rua Ferreira Borges 88", "Coimbra", "3000-179")
	startTime := c.clock.Tick().Add(time.Hour) // movement starts after system entry

	v := c.issueStock(c.stockDraft(domain.GT, c.f.Cust["C003"], date, from, to, startTime,
		c.line("P011", 10, date),
	), domain.IssueOptions{})
	printJSON("GT valorizada", v)
	stockSummary("5.11a (GT valorizada)", v)
	expectDoc("5.11a", v.Totals, 534.00, 69.42, 603.42)

	// Non-valued GR — UnitPrice=0 with Tax=nil yields gross 0 (regras §3.6 / I-H7).
	nv := c.issueStock(c.stockDraft(domain.GR, c.f.Cust["C003"], date, from, to, startTime.Add(30*time.Minute),
		newLine(c.f.Cat["P011"].p, 5, 0, nil, date),
	), domain.IssueOptions{})
	printJSON("GR não valorizada (UnitPrice=0, Tax=nil)", nv)
	stockSummary("5.11b (GR não valorizada)", nv)
	expectDoc("5.11b", nv.Totals, 0.00, 0.00, 0.00)
}

// ─── 5.12 ───────────────────────────────────────────────────────────────────

func Scenario512(c *Ctx, date time.Time) {
	banner("5.12", "Orçamento (OR)")
	c.atDay(date)
	draft := c.workDraft(domain.OR, c.f.Cust["C003"], date,
		c.line("P010", 8, date),
		c.line("P011", 2, date),
	)
	doc := c.issueWork(draft, domain.IssueOptions{})
	printJSON("OR issued", doc)
	workSummary("5.12", doc)
	expectDoc("5.12", doc.Totals, 706.80, 151.88, 858.68)
}

// ─── 5.13 ───────────────────────────────────────────────────────────────────

func Scenario513(c *Ctx) {
	banner("5.13", "Um exemplo de cada um dos restantes tipos de documento")

	loc := c.clock.Now().Location()
	d := func(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, loc) }

	c.scenario513FR(d(6, 15))
	c.scenario513ND(d(6, 16))
	c.scenario513GA(d(5, 25))
	c.scenario513GC(d(5, 26))
	c.scenario513GD(d(6, 17))
	c.scenario513Working(domain.NE, "Nota de encomenda", "C002", d(6, 2), 507.60, 95.39, 602.99,
		c.line("P003", 24, d(6, 2)), c.line("P004", 12, d(6, 2)))
	c.scenario513Working(domain.CM, "Consulta de mesa", "C005", d(6, 18), 15.95, 2.78, 18.73,
		c.line("P003", 1, d(6, 18)), c.line("P006", 2, d(6, 18)), c.line("P007", 1, d(6, 18)))
	c.scenario513Working(domain.FC, "Fatura de consignação", "C003", d(6, 19), 441.00, 101.43, 542.43,
		c.line("P004", 18, d(6, 19)))
	c.scenario513Working(domain.FO, "Folha de obra", "C002", d(5, 28), 280.00, 64.40, 344.40,
		c.line("P013", 8, d(5, 28)))
	c.scenario513Working(domain.OU, "Outros", "C003", d(6, 20), 150.00, 34.50, 184.50,
		c.line("P010", 2, d(6, 20)))
	c.scenario513RC(d(6, 15))
	c.scenario513RG(d(6, 16))
}

func (c *Ctx) scenario513FR(date time.Time) {
	fmt.Println("\n--- 5.13 · Fatura-recibo (FR) ---")
	c.atDay(date)
	draft := c.salesDraft(domain.FR, c.f.Cust["C001"], date, domain.SalesInvoiceFields{},
		c.line("P007", 2, date),
	)
	draft.CalculateTotals()
	draft.Payments = []domain.FRPayment{{
		Mechanism: domain.PaymentMechanismCash,
		Amount:    draft.Totals.GrossTotal,
		Date:      date,
	}}
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("FR issued", doc)
	salesSummary("5.13 FR", doc)
	expectSales("5.13 FR", doc, 6.90, 1.59, 8.49)
}

func (c *Ctx) scenario513ND(date time.Time) {
	fmt.Println("\n--- 5.13 · Nota de débito (ND) — acerto de valor sobre 5.6 ---")
	c.atDay(date)
	// F-SAFT-19: an ND adjusts value only — its line must mirror a product+quantity
	// already on the referenced invoice. 5.6 (FT2026/3) carries P004 at qty 2; the
	// €12.50 unit price is the upward acerto → net 25.00. (The reviewed dataset
	// pointed at FT2026/1, but that invoice is cancelled — FT2026/3 is the live target.)
	line := newLine(c.f.Cat["P004"].p, 2, 12.50, taxNOR(), date)
	line.Quantity = doc56.Lines[3].Quantity // exact match to the originating P004 line
	line.References = []domain.DocReference{{
		Reference: doc56.Number.Format(),
		Reason:    "Acerto de valor",
	}}
	draft := c.salesDraft(domain.ND, c.f.Cust["C002"], date, domain.SalesInvoiceFields{}, line)
	doc := c.issueSales(draft, domain.IssueOptions{})
	printJSON("ND issued (references FT2026/3, acerto de valor)", doc)
	salesSummary("5.13 ND", doc)
	expectSales("5.13 ND", doc, 25.00, 5.75, 30.75)
}

func (c *Ctx) scenario513GA(date time.Time) {
	fmt.Println("\n--- 5.13 · Guia de movimentação de ativos próprios (GA) ---")
	c.atDay(date)
	from := mustShip("Armazém Benedita", "Benedita", "2475-123")
	to := mustShip("Loja Lisboa", "Lisboa", "1200-194")
	// Own-asset movement: no external customer (SELF stands in), NS tax (P012 default).
	draft := c.stockDraft(domain.GA, c.f.Cust["SELF"], date, from, to, c.clock.Now().Add(time.Hour),
		c.line("P012", 1, date),
	)
	doc := c.issueStock(draft, domain.IssueOptions{})
	printJSON("GA issued", doc)
	stockSummary("5.13 GA", doc)
	expectDoc("5.13 GA", doc.Totals, 1850.00, 0.00, 1850.00)
}

func (c *Ctx) scenario513GC(date time.Time) {
	fmt.Println("\n--- 5.13 · Guia de consignação (GC) ---")
	c.atDay(date)
	from := mustShip("Armazém Benedita", "Benedita", "2475-123")
	to := mustShip("Mercearia Central", "Coimbra", "3000-179")
	// Consignment: VAT deferred to the consignment invoice (M25), so NS on the guia.
	draft := c.stockDraft(domain.GC, c.f.Cust["C003"], date, from, to, c.clock.Now().Add(time.Hour),
		c.nsLine("P004", 24, domain.M25, "Mercadorias à consignação", date),
	)
	doc := c.issueStock(draft, domain.IssueOptions{})
	printJSON("GC issued", doc)
	stockSummary("5.13 GC", doc)
	expectDoc("5.13 GC", doc.Totals, 588.00, 0.00, 588.00)
}

func (c *Ctx) scenario513GD(date time.Time) {
	fmt.Println("\n--- 5.13 · Guia de devolução (GD) ---")
	c.atDay(date)
	from := mustShip("Mercearia Central", "Coimbra", "3000-179")
	to := mustShip("Armazém Benedita", "Benedita", "2475-123")
	draft := c.stockDraft(domain.GD, c.f.Cust["C003"], date, from, to, c.clock.Now().Add(time.Hour),
		c.nsLine("P011", 2, domain.M99, "Devolução de mercadoria", date),
	)
	doc := c.issueStock(draft, domain.IssueOptions{})
	printJSON("GD issued", doc)
	stockSummary("5.13 GD", doc)
	expectDoc("5.13 GD", doc.Totals, 106.80, 0.00, 106.80)
}

func (c *Ctx) scenario513Working(dt domain.DocumentType, title, custID string, date time.Time, wantNet, wantTax, wantGross float64, lines ...domain.DocumentLine) {
	fmt.Printf("\n--- 5.13 · %s (%s) ---\n", title, dt)
	c.atDay(date)
	doc := c.issueWork(c.workDraft(dt, c.f.Cust[custID], date, lines...), domain.IssueOptions{})
	printJSON(string(dt)+" issued", doc)
	workSummary("5.13 "+string(dt), doc)
	expectDoc("5.13 "+string(dt), doc.Totals, wantNet, wantTax, wantGross)
}

func (c *Ctx) scenario513RC(date time.Time) {
	fmt.Println("\n--- 5.13 · Recibo (RC) liquida a fatura 5.6 ---")
	c.atDay(date)
	// One settlement line per VAT-rate bucket of invoice 5.6, so the line-derived
	// VAT (and thus the QR I/J/K blocks and the SAF-T Payment lines) reconciles
	// with TaxPayable. Collapsing the whole gross onto a single NOR line would make
	// the line's implied VAT contradict field N — rejected by IssuePayment's RC guard.
	lines := make([]domain.PaymentLine, 0, len(doc56.Totals.Breakdown))
	for i, e := range doc56.Totals.Breakdown {
		lines = append(lines, domain.PaymentLine{
			LineNumber: i + 1,
			SourceDocuments: []domain.SourceDocumentID{{
				OriginatingON: doc56.Number.Format(),
				InvoiceDate:   doc56.Date,
				Description:   "Liquidação integral",
			}},
			Movement: domain.CreditAmount{Value: e.Base + e.Tax},
			Tax:      must(domain.NewVATLineTax(e.Region, e.Category, e.ExemptionCode, e.ExemptionDescription)),
		})
	}
	draft := &domain.PaymentDraft{
		Type:            domain.RC,
		TransactionDate: date,
		Customer:        c.f.Cust["C002"],
		SourceID:        c.f.IssuerUser.Email,
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismMultibanco,
			Amount:    doc56.Totals.GrossTotal,
			Date:      date,
		}},
		Lines: lines,
	}
	totals := domain.PaymentTotals{
		NetTotal:   doc56.Totals.NetTotal,
		TaxPayable: doc56.Totals.TaxTotal,
		GrossTotal: doc56.Totals.GrossTotal,
	}
	doc := c.issuePayment(draft, totals, domain.IssueOptions{})
	printJSON("RC issued", doc)
	paymentSummary("5.13 RC", doc)
	expectTotals("5.13 RC", doc.PaymentTotals.NetTotal, doc.PaymentTotals.TaxPayable, doc.PaymentTotals.GrossTotal, 256.27, 18.44, 274.71)
}

func (c *Ctx) scenario513RG(date time.Time) {
	fmt.Println("\n--- 5.13 · Outro recibo (RG) — adiantamento/sinal ---")
	c.atDay(date)
	advance := must(domain.NewMoney(50.00))
	line := domain.PaymentLine{
		LineNumber: 1,
		SourceDocuments: []domain.SourceDocumentID{{
			OriginatingON: "Adiantamento " + date.Format("2006-01-02"),
			InvoiceDate:   date,
			Description:   "Adiantamento por conta de serviços futuros",
		}},
		Movement: domain.CreditAmount{Value: advance},
	}
	draft := &domain.PaymentDraft{
		Type:            domain.RG,
		TransactionDate: date,
		Customer:        c.f.Cust["C001"],
		SourceID:        c.f.IssuerUser.Email,
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismBankTransfer,
			Amount:    advance,
			Date:      date,
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
	expectTotals("5.13 RG", doc.PaymentTotals.NetTotal, doc.PaymentTotals.TaxPayable, doc.PaymentTotals.GrossTotal, 50.00, 0.00, 50.00)
}
