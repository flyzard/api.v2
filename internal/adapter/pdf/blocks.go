package pdf

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Palette. Muted grays carry the layout; red is reserved for ANULADO.
var (
	red      = &props.Color{Red: 200, Green: 30, Blue: 30}
	ink      = &props.Color{Red: 40, Green: 40, Blue: 40}
	gray     = &props.Color{Red: 110, Green: 110, Blue: 110}
	hairline = &props.Color{Red: 200, Green: 200, Blue: 200}
	paleBg   = &props.Color{Red: 242, Green: 242, Blue: 242}
)

// Shared table cell styles (lines table, payment lines table).
var (
	tableHdr       = props.Text{Size: 8, Style: fontstyle.Bold, Top: 1.5, Left: 1, Right: 1}
	tableRightHdr  = props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right, Top: 1.5, Left: 1, Right: 1}
	tableCell      = props.Text{Size: 8, Top: 1.2, Left: 1, Right: 1, Color: ink}
	tableRightCell = props.Text{Size: 8, Align: align.Right, Top: 1.2, Left: 1, Right: 1, Color: ink}

	tableHdrStyle  = &props.Cell{BackgroundColor: paleBg}
	tableLineStyle = &props.Cell{BorderType: border.Bottom, BorderColor: hairline, BorderThickness: 0.2}
)

// divider is the thin horizontal rule between layout zones.
func divider(height float64) core.Row {
	return line.NewRow(height, props.Line{Color: hairline, Thickness: 0.3})
}

// docIdentity is what the page header prints about the document itself.
type docIdentity struct {
	Type        domain.DocumentType
	Number      domain.DocNumber
	Date        time.Time
	DueDate     *time.Time
	SystemEntry *time.Time // working docs print this (date+time); nil = not printed
}

// headerRows builds the per-page document header: seller block (+ optional
// logo) on the left, document title/number/date/copy on the right, then the
// customer block. Registered as the maroto header, so it repeats on every page.
func headerRows(m Meta, id docIdentity, cust domain.Customer) []core.Row {
	var nameCols []core.Col
	nameSize := 7
	if len(m.LogoPNG) > 0 {
		nameCols = append(nameCols,
			image.NewFromBytesCol(2, m.LogoPNG, extension.Png, props.Rect{Percent: 100}))
		nameSize = 5
	}
	nameCols = append(nameCols,
		text.NewCol(nameSize, m.Seller.Name, props.Text{Size: 13, Style: fontstyle.Bold}),
		text.NewCol(5, docTypeLabel(id.Type),
			props.Text{Size: 13, Style: fontstyle.Bold, Align: align.Right}),
	)

	sellerLine := fmt.Sprintf("NIF: %s · %s", m.Seller.TaxID, fmtAddress(domain.Address{
		AddressDetail: m.Seller.Address, PostalCode: m.Seller.PostalCode, City: m.Seller.City}))
	contacts := ""
	if m.Seller.Phone != "" {
		contacts = "Tel: " + m.Seller.Phone
	}
	if m.Seller.Email != "" {
		if contacts != "" {
			contacts += " · "
		}
		contacts += m.Seller.Email
	}
	dateLine := "Data: " + fmtDate(id.Date)
	if id.DueDate != nil {
		dateLine = "Vencimento: " + fmtDate(*id.DueDate) + " · " + dateLine
	}
	var sysLine string
	if id.SystemEntry != nil && !id.SystemEntry.IsZero() {
		sysLine = "Entrada no sistema: " + fmtDateTime(*id.SystemEntry)
	}

	rows := []core.Row{
		row.New().Add(nameCols...),
		row.New(4).Add(
			text.NewCol(7, sellerLine, props.Text{Size: 7, Color: gray}),
			text.NewCol(5, id.Number.Format(),
				props.Text{Size: 10, Style: fontstyle.Bold, Align: align.Right}),
		),
		row.New(4).Add(
			text.NewCol(7, contacts, props.Text{Size: 7, Color: gray}),
			text.NewCol(5, m.Copy.label(),
				props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right}),
		),
		row.New(3.5).Add(
			col.New(7),
			text.NewCol(5, dateLine, props.Text{Size: 7, Color: gray, Align: align.Right}),
		),
	}
	if sysLine != "" {
		rows = append(rows, row.New(3.5).Add(
			col.New(7),
			text.NewCol(5, sysLine, props.Text{Size: 7, Color: gray, Align: align.Right}),
		))
	}
	rows = append(rows, divider(3))
	rows = append(rows, customerRows(cust, id.Type)...)
	rows = append(rows, row.New(3))
	return rows
}

func customerRows(c domain.Customer, dt domain.DocumentType) []core.Row {
	label := row.New(4).Add(text.NewCol(12, "CLIENTE",
		props.Text{Size: 6.5, Style: fontstyle.Bold, Color: gray, Top: 1}))
	bold := func(s string) core.Row {
		return row.New(4.5).Add(text.NewCol(12, s, props.Text{Size: 9, Style: fontstyle.Bold}))
	}
	hasNIF := !c.IsAnonymous() && c.CustomerTaxID != domain.FinalConsumerNIF
	if dt == domain.FS {
		if !hasNIF {
			return []core.Row{label, bold("NIF: Consumidor final")}
		}
		return []core.Row{label, bold("NIF: " + string(c.CustomerTaxID))}
	}
	if c.IsAnonymous() {
		return []core.Row{label, bold("NIF: Consumidor final")}
	}
	name := c.CompanyName
	if hasNIF {
		name += " · NIF: " + string(c.CustomerTaxID)
	}
	return []core.Row{
		label,
		bold(name),
		row.New(3.5).Add(text.NewCol(12, fmtAddress(c.BillingAddress),
			props.Text{Size: 8, Color: gray})),
	}
}

func fmtAddress(a domain.Address) string {
	return fmt.Sprintf("%s, %s %s", a.AddressDetail, a.PostalCode, a.City)
}

// cancelledRows is the red banner stamped on Status == "A" documents.
func cancelledRows(reason string) []core.Row {
	label := "ANULADO"
	if reason != "" {
		label += " — " + reason
	}
	return []core.Row{
		row.New(10).Add(text.NewCol(12, label,
			props.Text{Size: 14, Style: fontstyle.Bold, Align: align.Center, Color: red, Top: 2})),
	}
}

// linesTable renders the items table: shaded column header, then one bordered row per line.
func linesTable(lines []domain.DocumentLine) []core.Row {
	rows := make([]core.Row, 0, 2*len(lines)+1)
	rows = append(rows, row.New(6.5).Add(
		text.NewCol(2, "Código", tableHdr),
		text.NewCol(3, "Descrição", tableHdr),
		text.NewCol(1, "Qtd.", tableRightHdr),
		text.NewCol(2, "Preço unit.", tableRightHdr),
		text.NewCol(1, "Desc.", tableRightHdr),
		text.NewCol(1, "IVA", tableRightHdr),
		text.NewCol(2, "Total", tableRightHdr),
	).WithStyle(tableHdrStyle))
	for _, l := range lines {
		// Auto-height row so long descriptions wrap instead of overflowing;
		// the hairline lives on a thin spacer row to keep padding under the text.
		rows = append(rows,
			row.New().Add(
				text.NewCol(2, l.Product.ProductCode, tableCell),
				text.NewCol(3, l.Product.ProductDescription, tableCell),
				text.NewCol(1, l.Quantity.String(), tableRightCell),
				text.NewCol(2, fmtEUR(l.UnitPrice), tableRightCell),
				text.NewCol(1, discountLabel(l.Discount), tableRightCell),
				text.NewCol(1, lineTaxLabel(l.Tax), tableRightCell),
				text.NewCol(2, fmtEUR(l.LineNetAmount()), tableRightCell),
			),
			row.New(1.4).WithStyle(tableLineStyle),
		)
	}
	return rows
}

func discountLabel(d domain.Discount) string {
	switch v := d.(type) {
	case domain.PercentDiscount:
		return fmtPercent(v.Rate)
	case domain.AmountDiscount:
		return fmtEUR(v.Amount)
	default:
		return ""
	}
}

func lineTaxLabel(t domain.LineTax) string {
	switch v := t.(type) {
	case domain.VATTax:
		if v.Rate.Category == domain.TaxExempt {
			code := v.Rate.Exemption
			if code.IsReverseCharge() {
				return "Autoliq. (" + string(code) + ")"
			}
			return "Isento (" + string(code) + ")"
		}
		return fmtPercent(v.Rate.Value)
	case domain.StampTax:
		return "IS"
	default:
		return ""
	}
}

// referencesRows prints the rectified-document references (mandatory content
// for NC/ND per CIVA Art. 36.º n.º 6).
func referencesRows(lines []domain.DocumentLine) []core.Row {
	var rows []core.Row
	for _, l := range lines {
		for _, r := range l.References {
			label := "Referência: " + r.Reference
			if r.Reason != "" {
				label += " — Motivo: " + r.Reason
			}
			rows = append(rows, row.New(4).Add(text.NewCol(12, label, props.Text{Size: 7, Color: gray})))
		}
	}
	return rows
}

// shippingRows prints load/unload places and movement times when the document
// doubles as a transport document (sales invoices) or is one (guias).
func shippingRows(from, to *domain.ShippingPoint, start, end *time.Time) []core.Row {
	var rows []core.Row
	point := func(label string, sp *domain.ShippingPoint) {
		if sp == nil || sp.Address == nil {
			return
		}
		rows = append(rows, row.New(4).Add(text.NewCol(12,
			label+": "+fmtAddress(*sp.Address), props.Text{Size: 7, Color: gray})))
	}
	point("Local de carga", from)
	point("Local de descarga", to)
	if start != nil && !start.IsZero() {
		t := "Início do transporte: " + start.Format("2006-01-02 15:04")
		if end != nil && !end.IsZero() {
			t += " · Fim: " + end.Format("2006-01-02 15:04")
		}
		rows = append(rows, row.New(4).Add(text.NewCol(12, t, props.Text{Size: 7, Color: gray})))
	}
	return rows
}

// taxEntryLabel names one breakdown row using only frozen document data: the
// exemption code for exempt/reverse-charge entries, the matching lines' frozen
// rate otherwise. domain.GetTaxRate is deliberately not consulted — a reprint
// must not change when the statutory rate table does.
//
// The breakdown is keyed by (Region, Category, Exemption), not rate value, and
// OUT lines carry caller-supplied rates — one entry can aggregate mixed rates.
// The rate prints only when every matching line agrees on it.
func taxEntryLabel(e domain.TaxBreakdownEntry, lines []domain.DocumentLine) string {
	if e.ExemptionCode != "" {
		if e.ExemptionCode.IsReverseCharge() {
			return "IVA autoliquidação (" + string(e.ExemptionCode) + ")"
		}
		return "Isento (" + string(e.ExemptionCode) + ")"
	}
	var rate domain.Percent
	found := false
	for _, l := range lines {
		v, ok := l.Tax.(domain.VATTax)
		if !ok || v.Rate.Region != e.Region || v.Rate.Category != e.Category || v.Rate.Exemption != "" {
			continue
		}
		if found && v.Rate.Value != rate {
			return string(e.Category) + " (" + string(e.Region) + ")" // mixed rates in one bucket
		}
		rate, found = v.Rate.Value, true
	}
	if found {
		return "IVA " + fmtPercent(rate) + " (" + string(e.Region) + ")"
	}
	return string(e.Category) + " (" + string(e.Region) + ")"
}

// vatCategoryRank orders VAT summary rows from exempt up to the normal rate
// (AT cert review). Ranks by rate TIER via the category enum, not the live rate
// value, so a reprint never shifts when the statutory table changes; OUT has no
// canonical tier and sorts last.
func vatCategoryRank(c domain.TaxCategory) int {
	switch c {
	case domain.TaxExempt:
		return 0
	case domain.TaxReduced:
		return 1
	case domain.TaxIntermediate:
		return 2
	case domain.TaxNormal:
		return 3
	default: // TaxOther / unknown
		return 4
	}
}

// displayBreakdown returns a re-sorted COPY for the printed VAT summary only.
// The domain order (sortTaxBreakdown) feeds the frozen QR and persisted
// Totals.Breakdown and must NOT change, so we sort a clone here.
func displayBreakdown(b domain.TaxBreakdown) domain.TaxBreakdown {
	out := slices.Clone(b)
	slices.SortFunc(out, func(x, y domain.TaxBreakdownEntry) int {
		return cmp.Or(
			cmp.Compare(x.Region, y.Region),
			cmp.Compare(vatCategoryRank(x.Category), vatCategoryRank(y.Category)),
			cmp.Compare(x.ExemptionCode, y.ExemptionCode),
		)
	})
	return out
}

// totalEntry is one label/value pair on the totals block.
type totalEntry struct {
	label string
	value string
	bold  bool
}

func globalDiscountSum(lines []domain.DocumentLine) domain.Money {
	var sum domain.Money
	for _, l := range lines {
		sum += l.GlobalDiscountShare
	}
	return sum
}

// salesTotals lists the totals block of invoices/work documents/movements.
// The global discount (AT cert §5.7) is informational — already folded into
// line totals and Total líquido — and gets a footnote in summaryAndTotalsRows.
func salesTotals(t domain.Totals, wh []domain.WithholdingTax, lines []domain.DocumentLine) []totalEntry {
	var entries []totalEntry
	if gd := globalDiscountSum(lines); gd != 0 {
		entries = append(entries, totalEntry{"Desconto global ¹", fmtEUR(gd), false})
	}
	entries = append(entries,
		totalEntry{"Total líquido", fmtEUR(t.NetTotal), false},
		totalEntry{"Total IVA", fmtEUR(t.TaxTotal), false},
	)
	if t.StampDuty != 0 {
		entries = append(entries, totalEntry{"Imposto do Selo", fmtEUR(t.StampDuty), false})
	}
	entries = append(entries, totalEntry{"Total", fmtEUR(t.GrossTotal), true})
	for _, w := range wh {
		if w.Amount == 0 {
			continue
		}
		entries = append(entries, totalEntry{withholdingLabel(w), "-" + fmtEUR(w.Amount), false})
	}
	if t.AmountPayable != t.GrossTotal {
		entries = append(entries, totalEntry{"Total a pagar", fmtEUR(t.AmountPayable), true})
	}
	return entries
}

// withholdingLabel names one retention entry on the totals block.
func withholdingLabel(w domain.WithholdingTax) string {
	if w.Description != "" {
		return "Retenção na fonte (" + w.Description + ")"
	}
	return "Retenção na fonte"
}

// summaryAndTotalsRows lays the tax summary (left, 7 grid cols) beside the
// totals block (right, 5 grid cols), then appends the exemption-description
// and global-discount footnotes. Either side may be empty.
func summaryAndTotalsRows(b domain.TaxBreakdown, lines []domain.DocumentLine, totals []totalEntry) []core.Row {
	b = displayBreakdown(b) // print-only order; domain Breakdown (frozen QR) untouched
	var left [][]core.Col
	if len(b) > 0 {
		left = append(left, []core.Col{
			text.NewCol(3, "Resumo de impostos", props.Text{Size: 8, Style: fontstyle.Bold}),
			text.NewCol(2, "Base", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right}),
			text.NewCol(2, "IVA", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right}),
		})
		for _, e := range b {
			left = append(left, []core.Col{
				text.NewCol(3, taxEntryLabel(e, lines), props.Text{Size: 7.5, Color: ink}),
				text.NewCol(2, fmtEUR(e.Base), props.Text{Size: 7.5, Align: align.Right, Color: ink}),
				text.NewCol(2, fmtEUR(e.Tax), props.Text{Size: 7.5, Align: align.Right, Color: ink}),
			})
		}
	}

	var right [][]core.Col
	for _, e := range totals {
		style, size := fontstyle.Normal, 8.0
		if e.bold {
			style, size = fontstyle.Bold, 9.0
		}
		right = append(right, []core.Col{
			text.NewCol(3, e.label, props.Text{Size: size, Style: style, Align: align.Right}),
			text.NewCol(2, e.value, props.Text{Size: size, Style: style, Align: align.Right}),
		})
	}

	rows := []core.Row{row.New(2)}
	for i := 0; i < len(left) || i < len(right); i++ {
		l := []core.Col{col.New(7)}
		if i < len(left) {
			l = left[i]
		}
		r := []core.Col{col.New(5)}
		if i < len(right) {
			r = right[i]
		}
		rows = append(rows, row.New(4.5).Add(append(l, r...)...))
	}

	note := func(s string) {
		rows = append(rows, row.New(3.5).Add(text.NewCol(12, s, props.Text{Size: 6.5, Color: gray})))
	}
	seen := map[domain.Exemption]bool{}
	for _, e := range b {
		if e.ExemptionCode == "" || seen[e.ExemptionCode] || e.ExemptionDescription == "" {
			continue
		}
		seen[e.ExemptionCode] = true
		note(string(e.ExemptionCode) + " — " + e.ExemptionDescription)
	}
	if globalDiscountSum(lines) != 0 {
		note("¹ Desconto global já refletido nos valores das linhas e no total líquido.")
	}
	return rows
}

// frPaymentRows prints the FR (Fatura-Recibo) settlement entries.
func frPaymentRows(ps []domain.FRPayment) []core.Row {
	var rows []core.Row
	for _, p := range ps {
		label := fmt.Sprintf("Pagamento: %s · %s · %s",
			mechanismLabel(p.Mechanism), fmtEUR(p.Amount), fmtDate(p.Date))
		rows = append(rows, row.New(4).Add(text.NewCol(12, label, props.Text{Size: 7, Color: gray})))
	}
	return rows
}

// regimeRows prints the conditional legal regime mentions.
func regimeRows(r domain.SpecialRegimes) []core.Row {
	var rows []core.Row
	mention := func(s string) {
		rows = append(rows, row.New(4).Add(text.NewCol(12, s,
			props.Text{Size: 8, Style: fontstyle.Bold})))
	}
	if r.CashVAT {
		mention("IVA — regime de caixa")
	}
	if r.SelfBilling {
		mention("Autofaturação")
	}
	return rows
}

// thirdPartyRows prints the Portaria 363/2010 mention for guias issued on behalf
// of third parties (DocumentStatus 'T'). Slogan only — the third-party identity
// is not modelled (StockMovementFields carries no party fields).
func thirdPartyRows() []core.Row {
	return []core.Row{
		row.New(4).Add(text.NewCol(12, "Por conta de terceiros",
			props.Text{Size: 8, Style: fontstyle.Bold})),
	}
}

// currencyRows prints the original-currency note for non-EUR documents.
func currencyRows(c *domain.Currency) []core.Row {
	if c == nil {
		return nil
	}
	return []core.Row{
		row.New(4).Add(text.NewCol(12, currencyLabel(*c), props.Text{Size: 7, Color: gray})),
	}
}
