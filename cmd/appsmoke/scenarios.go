package main

import (
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
)

// ─── 5.1 ────────────────────────────────────────────────────────────────────

func Scenario51(c *Ctx, date time.Time) {
	banner("5.1", "Fatura simplificada (art. 40.º CIVA) — cliente com NIF")
	c.atDay(date)
	in := c.salesInput(app.DocFS, c.f.Cust["C001"], date,
		c.line("P007", 2, date),
	)
	doc := c.issueSales(in)
	printJSON("FS issued", doc)
	salesSummary("5.1", doc)
	expectDoc("5.1", doc, 690, 159, 849)
}

// ─── 5.2 ────────────────────────────────────────────────────────────────────

func Scenario52(c *Ctx, date time.Time) {
	banner("5.2", "Fatura (art. 36.º CIVA) anulada — PDF + SAF-T DocumentStatus")
	c.atDay(date)
	in := c.salesInput(app.DocFT, c.f.Cust["C002"], date,
		c.line("P003", 12, date),
		c.line("P004", 6, date),
	)
	in.PaymentTermsDays = days(30)
	doc := c.issueSales(in)
	printJSON("FT issued (pre-cancellation)", doc)
	expectDoc("5.2", doc, 25380, 4769, 30149)

	doc = c.cancel(doc.Number, "Anulação a pedido")
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
	doc53 app.IssuedView
	doc54 app.IssuedView
	doc56 app.IssuedView
)

func Scenario53(c *Ctx, date time.Time) {
	banner("5.3", "Pró-forma para conferência (PF) — origem do 5.4")
	c.atDay(date)
	in := c.workInput(app.DocPF, c.f.Cust["C003"], date,
		c.line("P011", 4, date),
		c.line("P003", 6, date),
	)
	doc53 = c.issueWork(in)
	printJSON("PF issued", doc53)
	workSummary("5.3", doc53)
	expectDoc("5.3", doc53, 26700, 3471, 30171)
}

// ─── 5.4 ────────────────────────────────────────────────────────────────────

func Scenario54(c *Ctx, date time.Time) {
	banner("5.4", "Fatura baseada na pró-forma 5.3 — gera OrderReferences")
	c.atDay(date)
	line1 := c.line("P011", 4, date)
	line1.OrderReferences = []app.OrderRefInput{{
		OriginatingON: doc53.Number,
		OrderDate:     doc53.Date,
	}}

	in := c.salesInput(app.DocFT, c.f.Cust["C003"], date,
		line1,
		c.line("P003", 6, date),
	)
	in.PaymentTermsDays = days(30)
	doc54 = c.issueSales(in)
	printJSON("FT issued with OrderReferences → PF", doc54)
	expectDoc("5.4", doc54, 26700, 3471, 30171)

	// MarkWorkBilled reads Clock.Now() (no tick of its own); the legacy domain call
	// consumed a tick, so keep one here to preserve SystemEntryDate ordering.
	c.clock.Tick()
	billed, err := c.svc.Invoicing.MarkWorkBilled(c.ctx, c.tenant, doc53.Number, doc54.Number)
	if err != nil {
		log.Fatalf("mark PF billed: %v", err)
	}
	c.store.record(billed)
	printJSON("PF after MarkBilled (Status=F)", billed)
	salesSummary("5.4", doc54)
}

// ─── 5.5 ────────────────────────────────────────────────────────────────────

func Scenario55(c *Ctx, date time.Time) {
	banner("5.5", "Nota de crédito sobre a fatura 5.4 — References ao nível da linha")
	c.atDay(date)
	line := c.line("P011", 1, date)
	line.References = []app.DocRefInput{{
		Reference: doc54.Number,
		Reason:    "Devolução de 1 caixa de vinho",
	}}

	in := c.salesInput(app.DocNC, c.f.Cust["C003"], date, line)
	doc := c.issueSales(in)
	printJSON("NC issued with line References → FT", doc)
	salesSummary("5.5", doc)
	expectDoc("5.5", doc, 5340, 694, 6034)
}

// ─── 5.6 ────────────────────────────────────────────────────────────────────

func Scenario56(c *Ctx, date time.Time) {
	banner("5.6", "Fatura com 4 linhas — Reduzida / Isento (M07) / Intermédia / Normal")
	c.atDay(date)
	in := c.salesInput(app.DocFT, c.f.Cust["C002"], date,
		c.line("P001", 3, date),
		c.line("P002", 1, date), // isento M07 (catalogue default)
		c.line("P003", 6, date),
		c.line("P004", 2, date),
	)
	in.PaymentTermsDays = days(30)
	doc56 = c.issueSales(in)
	printJSON("FT 4 linhas (RED/ISE/INT/NOR)", doc56)
	salesSummary("5.6", doc56)
	expectDoc("5.6", doc56, 25627, 1844, 27471)
}

// ─── 5.7 ────────────────────────────────────────────────────────────────────

func Scenario57(c *Ctx, date time.Time) {
	banner("5.7", "Fatura — desconto de linha 8.8% + desconto global (Settlement)")
	c.atDay(date)
	// L1: qty 100 × €0.55 with an 8.8% line discount → net 50.16. L2: qty 10 ×
	// €1.80, net 18.00. The engine models the §5.7 global discount as COMMERCIAL:
	// GlobalDiscount prorates into the lines, reducing base + VAT and surfacing the
	// same amount as Settlement (Net 64.75 / Tax 14.89 / Gross 79.64 / Settlement 3.41).
	line1 := c.line("P005", 100, date)
	line1.Discount = &app.DiscountInput{Kind: "percent", Percent: 8.8}
	line2 := c.line("P006", 10, date)

	in := c.salesInput(app.DocFT, c.f.Cust["C002"], date, line1, line2)
	in.GlobalDiscount = &app.DiscountInput{Kind: "amount", AmountCents: 341}
	in.PaymentTermsDays = days(30)
	doc := c.issueSales(in)
	printJSON("FT issued (line discount 8.8% + global discount €3.41)", doc)
	salesSummary("5.7", doc)
	expectDoc("5.7", doc, 6475, 1489, 7964)
}

// ─── 5.8 ────────────────────────────────────────────────────────────────────

func Scenario58(c *Ctx, date time.Time) {
	banner("5.8", "Fatura em moeda estrangeira (USD)")
	c.atDay(date)
	in := c.salesInput(app.DocFT, c.f.Cust["C006"], date,
		c.line("P004", 3, date),
	)
	in.Currency = &app.CurrencyInput{Code: "USD", RateMicro: 1085000}
	in.PaymentTermsDays = days(30)
	doc := c.issueSales(in)
	printJSON("FT issued (EUR totals + Currency block)", doc)
	salesSummary("5.8", doc)
	expectDoc("5.8", doc, 7350, 1691, 9041)
}

// ─── 5.9 ────────────────────────────────────────────────────────────────────

func Scenario59(c *Ctx, date time.Time) {
	banner("5.9", "Cliente identificado sem NIF — GrossTotal < €1,00 — SystemEntryDate < 10h")
	c.atDay(date) // 09:00 + a few ticks → entry stays before 10:00
	in := c.salesInput(app.DocFS, c.f.Cust["C004"], date,
		c.line("P008", 12, date),
	)
	doc := c.issueSales(in)
	printJSON("FS issued — pequeno valor, manhã", doc)
	salesSummary("5.9", doc)
	expectDoc("5.9", doc, 60, 14, 74)
}

// ─── 5.10 ───────────────────────────────────────────────────────────────────

func Scenario510(c *Ctx, date time.Time) {
	banner("5.10", "Outro cliente identificado sem NIF")
	c.atDay(date)
	in := c.salesInput(app.DocFS, c.f.Cust["C005"], date,
		c.line("P007", 3, date),
	)
	doc := c.issueSales(in)
	printJSON("FS issued — outro cliente sem NIF", doc)
	salesSummary("5.10", doc)
	expectDoc("5.10", doc, 1035, 238, 1273)
}

// ─── 5.11 ───────────────────────────────────────────────────────────────────

func Scenario511(c *Ctx, date time.Time) {
	banner("5.11", "Guia de transporte valorizada (GT) + guia de remessa não valorizada (GR)")
	c.atDay(date)

	from := ship("Armazém do emitente", "Benedita", "2475-123")
	to := ship("Mercearia Central, Rua Ferreira Borges 88", "Coimbra", "3000-179")
	startTime := c.clock.Tick().Add(time.Hour) // movement starts after system entry

	v := c.issueStock(c.stockInput(app.DocGT, c.f.Cust["C003"], date, from, to, startTime,
		c.line("P011", 10, date),
	))
	printJSON("GT valorizada", v)
	stockSummary("5.11a (GT valorizada)", v)
	expectDoc("5.11a", v, 53400, 6942, 60342)

	// Non-valued GR — UnitPrice=0 with Tax=nil yields gross 0 (regras §3.6 / I-H7).
	nvLine := app.LineInput{
		ProductCode:        "P011",
		ProductType:        app.ProductGoods,
		ProductDescription: c.f.Cat["P011"].desc,
		ProductNumberCode:  "P011",
		Unit:               app.UnitPiece,
		Quantity:           5,
		UnitPriceCents:     0,
		TaxPointDate:       ymd(date),
		Tax:                nil,
	}
	nv := c.issueStock(c.stockInput(app.DocGR, c.f.Cust["C003"], date, from, to, startTime.Add(30*time.Minute),
		nvLine,
	))
	printJSON("GR não valorizada (UnitPrice=0, Tax=nil)", nv)
	stockSummary("5.11b (GR não valorizada)", nv)
	expectDoc("5.11b", nv, 0, 0, 0)
}

// ─── 5.12 ───────────────────────────────────────────────────────────────────

func Scenario512(c *Ctx, date time.Time) {
	banner("5.12", "Orçamento (OR)")
	c.atDay(date)
	in := c.workInput(app.DocOR, c.f.Cust["C003"], date,
		c.line("P010", 8, date),
		c.line("P011", 2, date),
	)
	doc := c.issueWork(in)
	printJSON("OR issued", doc)
	workSummary("5.12", doc)
	expectDoc("5.12", doc, 70680, 15188, 85868)
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
	c.scenario513Working(app.DocNE, "Nota de encomenda", "C002", d(6, 2), 50760, 9539, 60299,
		c.line("P003", 24, d(6, 2)), c.line("P004", 12, d(6, 2)))
	c.scenario513Working(app.DocCM, "Consulta de mesa", "C005", d(6, 18), 1595, 278, 1873,
		c.line("P003", 1, d(6, 18)), c.line("P006", 2, d(6, 18)), c.line("P007", 1, d(6, 18)))
	c.scenario513Working(app.DocFC, "Fatura de consignação", "C003", d(6, 19), 44100, 10143, 54243,
		c.line("P004", 18, d(6, 19)))
	c.scenario513Working(app.DocFO, "Folha de obra", "C002", d(5, 28), 28000, 6440, 34440,
		c.line("P013", 8, d(5, 28)))
	c.scenario513Working(app.DocOU, "Outros", "C003", d(6, 20), 15000, 3450, 18450,
		c.line("P010", 2, d(6, 20)))
	c.scenario513RC(d(6, 15))
	c.scenario513RG(d(6, 16))
}

func (c *Ctx) scenario513FR(date time.Time) {
	fmt.Println("\n--- 5.13 · Fatura-recibo (FR) ---")
	c.atDay(date)
	in := c.salesInput(app.DocFR, c.f.Cust["C001"], date,
		c.line("P007", 2, date),
	)
	// FR payment amount must equal the gross; preview the totals to read it.
	// PreviewTotals builds the sales draft (which validates IssuedBy), so set it.
	in.IssuedBy = c.f.IssuerUser
	tot, err := c.svc.Invoicing.PreviewTotals(c.ctx, c.tenant, in)
	if err != nil {
		log.Fatalf("preview FR totals: %v", err)
	}
	in.Payments = []app.FRPaymentInput{{
		Mechanism:   app.MechCash,
		AmountCents: tot.GrossCents,
		Date:        ymd(date),
	}}
	doc := c.issueSales(in)
	printJSON("FR issued", doc)
	salesSummary("5.13 FR", doc)
	expectDoc("5.13 FR", doc, 690, 159, 849)
}

func (c *Ctx) scenario513ND(date time.Time) {
	fmt.Println("\n--- 5.13 · Nota de débito (ND) — acerto de valor sobre 5.6 ---")
	c.atDay(date)
	// F-SAFT-19: an ND adjusts value only — its line must mirror a product+quantity
	// already on the referenced invoice. 5.6 (FT2026/3) carries P004 at qty 2; the
	// €12.50 unit price is the upward acerto → net 25.00.
	line := newLine(c.f.Cat["P004"], 0, 1250, taxNOR(), date)
	line.QuantityScaled = doc56.Lines[3].QuantityScaled // exact match to the originating P004 line
	line.References = []app.DocRefInput{{
		Reference: doc56.Number,
		Reason:    "Acerto de valor",
	}}
	in := c.salesInput(app.DocND, c.f.Cust["C002"], date, line)
	doc := c.issueSales(in)
	printJSON("ND issued (references FT2026/3, acerto de valor)", doc)
	salesSummary("5.13 ND", doc)
	expectDoc("5.13 ND", doc, 2500, 575, 3075)
}

func (c *Ctx) scenario513GA(date time.Time) {
	fmt.Println("\n--- 5.13 · Guia de movimentação de ativos próprios (GA) ---")
	c.atDay(date)
	from := ship("Armazém Benedita", "Benedita", "2475-123")
	to := ship("Loja Lisboa", "Lisboa", "1200-194")
	// Own-asset movement: no external customer (SELF stands in), NS tax (P012 default).
	in := c.stockInput(app.DocGA, c.f.Cust["SELF"], date, from, to, c.clock.Now().Add(time.Hour),
		c.line("P012", 1, date),
	)
	doc := c.issueStock(in)
	printJSON("GA issued", doc)
	stockSummary("5.13 GA", doc)
	expectDoc("5.13 GA", doc, 185000, 0, 185000)
}

func (c *Ctx) scenario513GC(date time.Time) {
	fmt.Println("\n--- 5.13 · Guia de consignação (GC) ---")
	c.atDay(date)
	from := ship("Armazém Benedita", "Benedita", "2475-123")
	to := ship("Mercearia Central", "Coimbra", "3000-179")
	// Consignment: VAT deferred to the consignment invoice (M25), so NS on the guia.
	in := c.stockInput(app.DocGC, c.f.Cust["C003"], date, from, to, c.clock.Now().Add(time.Hour),
		c.nsLine("P004", 24, app.ExemptM25, "Mercadorias à consignação", date),
	)
	doc := c.issueStock(in)
	printJSON("GC issued", doc)
	stockSummary("5.13 GC", doc)
	expectDoc("5.13 GC", doc, 58800, 0, 58800)
}

func (c *Ctx) scenario513GD(date time.Time) {
	fmt.Println("\n--- 5.13 · Guia de devolução (GD) ---")
	c.atDay(date)
	from := ship("Mercearia Central", "Coimbra", "3000-179")
	to := ship("Armazém Benedita", "Benedita", "2475-123")
	in := c.stockInput(app.DocGD, c.f.Cust["C003"], date, from, to, c.clock.Now().Add(time.Hour),
		c.nsLine("P011", 2, app.ExemptM99, "Devolução de mercadoria", date),
	)
	doc := c.issueStock(in)
	printJSON("GD issued", doc)
	stockSummary("5.13 GD", doc)
	expectDoc("5.13 GD", doc, 10680, 0, 10680)
}

func (c *Ctx) scenario513Working(dt, title, custID string, date time.Time, wantNet, wantTax, wantGross int64, lines ...app.LineInput) {
	fmt.Printf("\n--- 5.13 · %s (%s) ---\n", title, dt)
	c.atDay(date)
	doc := c.issueWork(c.workInput(dt, c.f.Cust[custID], date, lines...))
	printJSON(dt+" issued", doc)
	workSummary("5.13 "+dt, doc)
	expectDoc("5.13 "+dt, doc, wantNet, wantTax, wantGross)
}

func (c *Ctx) scenario513RC(date time.Time) {
	fmt.Println("\n--- 5.13 · Recibo (RC) liquida a fatura 5.6 ---")
	c.atDay(date)
	// One settlement line per VAT-rate bucket of invoice 5.6, so the line-derived
	// VAT (and thus the QR I/J/K blocks and the SAF-T Payment lines) reconciles
	// with TaxPayable. Collapsing the whole gross onto a single NOR line would make
	// the line's implied VAT contradict field N — rejected by IssuePayment's RC guard.
	lines := make([]app.PaymentLineInput, 0, len(doc56.Breakdown))
	for i, e := range doc56.Breakdown {
		lines = append(lines, app.PaymentLineInput{
			LineNumber: i + 1,
			SourceDocuments: []app.SourceDocInput{{
				OriginatingON: doc56.Number,
				InvoiceDate:   doc56.Date,
				Description:   "Liquidação integral",
			}},
			CreditCents: e.BaseCents + e.TaxCents,
			Tax: &app.LineTaxInput{
				Kind:            "VAT",
				Region:          e.Region,
				Category:        e.Category,
				ExemptionCode:   e.ExemptionCode,
				ExemptionReason: e.ExemptionDescription,
			},
		})
	}
	in := app.IssuePaymentInput{
		Type:            app.DocRC,
		TransactionDate: ymd(date),
		Customer:        c.f.Cust["C002"],
		SourceID:        c.f.IssuerUser.Email,
		Methods: []app.PaymentMethodInput{{
			Mechanism:   app.MechMultibanco,
			AmountCents: doc56.GrossCents,
			Date:        ymd(date),
		}},
		Lines:  lines,
		Totals: app.TotalsInput{NetCents: doc56.NetCents, TaxCents: doc56.TaxCents, GrossCents: doc56.GrossCents},
	}
	doc := c.issuePayment(in)
	printJSON("RC issued", doc)
	paymentSummary("5.13 RC", doc)
	expectTotals("5.13 RC", doc.NetCents, doc.TaxCents, doc.GrossCents, 25627, 1844, 27471)
}

func (c *Ctx) scenario513RG(date time.Time) {
	fmt.Println("\n--- 5.13 · Outro recibo (RG) — adiantamento/sinal ---")
	c.atDay(date)
	const advance int64 = 5000
	in := app.IssuePaymentInput{
		Type:            app.DocRG,
		TransactionDate: ymd(date),
		Customer:        c.f.Cust["C001"],
		SourceID:        c.f.IssuerUser.Email,
		Methods: []app.PaymentMethodInput{{
			Mechanism:   app.MechBankTransfer,
			AmountCents: advance,
			Date:        ymd(date),
		}},
		Lines: []app.PaymentLineInput{{
			LineNumber: 1,
			SourceDocuments: []app.SourceDocInput{{
				OriginatingON: "Adiantamento " + ymd(date),
				InvoiceDate:   ymd(date),
				Description:   "Adiantamento por conta de serviços futuros",
			}},
			CreditCents: advance,
		}},
		Totals: app.TotalsInput{NetCents: advance, TaxCents: 0, GrossCents: advance},
	}
	doc := c.issuePayment(in)
	printJSON("RG issued", doc)
	paymentSummary("5.13 RG", doc)
	expectTotals("5.13 RG", doc.NetCents, doc.TaxCents, doc.GrossCents, 5000, 0, 5000)
}
