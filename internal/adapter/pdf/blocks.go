package pdf

import (
	"fmt"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

var red = &props.Color{Red: 200, Green: 30, Blue: 30}

// Shared table cell styles (lines table, payment lines table).
var (
	tableHdr       = props.Text{Size: 8, Style: fontstyle.Bold}
	tableCell      = props.Text{Size: 8}
	tableRightHdr  = props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right}
	tableRightCell = props.Text{Size: 8, Align: align.Right}
)

// docIdentity is what the page header prints about the document itself.
type docIdentity struct {
	Type    domain.DocumentType
	Number  domain.DocNumber
	Date    time.Time
	DueDate *time.Time
}

// headerRows builds the per-page document header: seller block (+ optional
// logo), document title/number/date/copy mark, customer block.
func headerRows(m Meta, id docIdentity, cust domain.Customer) []core.Row {
	sellerCols := []core.Col{}
	sellerSize := 7
	if m.LogoPNG != nil {
		sellerCols = append(sellerCols,
			image.NewFromBytesCol(2, m.LogoPNG, extension.Png, props.Rect{Percent: 100}))
		sellerSize = 5
	}
	sellerCols = append(sellerCols,
		text.NewCol(sellerSize, m.Seller.Name, props.Text{Size: 11, Style: fontstyle.Bold}),
		text.NewCol(5, docTypeLabel(id.Type)+" "+id.Number.Format(),
			props.Text{Size: 11, Style: fontstyle.Bold, Align: align.Right}),
	)

	sellerLine2 := fmt.Sprintf("NIF: %s · %s, %s %s",
		m.Seller.TaxID, m.Seller.Address, m.Seller.PostalCode, m.Seller.City)
	if m.Seller.Phone != "" {
		sellerLine2 += " · Tel: " + m.Seller.Phone
	}
	if m.Seller.Email != "" {
		sellerLine2 += " · " + m.Seller.Email
	}
	dateLine := "Data: " + fmtDate(id.Date) + " · " + m.Copy.label()
	if id.DueDate != nil {
		dateLine = "Vencimento: " + fmtDate(*id.DueDate) + " · " + dateLine
	}

	rows := []core.Row{
		row.New(8).Add(sellerCols...),
		row.New(4).Add(
			text.NewCol(7, sellerLine2, props.Text{Size: 7}),
			text.NewCol(5, dateLine, props.Text{Size: 7, Align: align.Right}),
		),
	}
	rows = append(rows, customerRows(cust)...)
	return rows
}

// customerRows prints the customer block; the reserved anonymous pseudo-customer
// prints as "Consumidor final" (Portaria 302/2016).
func customerRows(c domain.Customer) []core.Row {
	if c.IsAnonymous() {
		return []core.Row{
			row.New(6).Add(text.NewCol(12, "Cliente: Consumidor final", props.Text{Size: 8, Top: 2})),
		}
	}
	addr := fmt.Sprintf("%s, %s %s",
		c.BillingAddress.AddressDetail, c.BillingAddress.PostalCode, c.BillingAddress.City)
	return []core.Row{
		row.New(5).Add(text.NewCol(12,
			"Cliente: "+c.CompanyName+" · NIF: "+string(c.CustomerTaxID),
			props.Text{Size: 8, Top: 2})),
		row.New(4).Add(text.NewCol(12, addr, props.Text{Size: 7})),
	}
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

// linesTable renders the items table: column header once, then one row per
// line. Amounts always positive (NC semantics live in the document type).
func linesTable(lines []domain.DocumentLine) []core.Row {
	rows := []core.Row{
		row.New(6).Add(
			text.NewCol(2, "Código", tableHdr),
			text.NewCol(3, "Descrição", tableHdr),
			text.NewCol(1, "Qtd.", tableRightHdr),
			text.NewCol(2, "Preço unit.", tableRightHdr),
			text.NewCol(1, "Desc.", tableRightHdr),
			text.NewCol(1, "IVA", tableRightHdr),
			text.NewCol(2, "Total", tableRightHdr),
		),
	}
	for _, l := range lines {
		unit := fmtEUR(l.UnitPrice)
		if l.TaxBase != nil {
			unit = fmtEUR(*l.TaxBase)
		}
		rows = append(rows, row.New(5).Add(
			text.NewCol(2, l.Product.ProductCode, tableCell),
			text.NewCol(3, l.Product.ProductDescription, tableCell),
			text.NewCol(1, l.Quantity.String(), tableRightCell),
			text.NewCol(2, unit, tableRightCell),
			text.NewCol(1, discountLabel(l.Discount), tableRightCell),
			text.NewCol(1, lineTaxLabel(l.Tax), tableRightCell),
			text.NewCol(2, fmtEUR(l.LineNetAmount()), tableRightCell),
		))
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
			rows = append(rows, row.New(4).Add(text.NewCol(12, label, props.Text{Size: 7})))
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
		a := sp.Address
		rows = append(rows, row.New(4).Add(text.NewCol(12,
			fmt.Sprintf("%s: %s, %s %s", label, a.AddressDetail, a.PostalCode, a.City),
			props.Text{Size: 7})))
	}
	point("Local de carga", from)
	point("Local de descarga", to)
	if start != nil && !start.IsZero() {
		t := "Início do transporte: " + start.Format("2006-01-02 15:04")
		if end != nil && !end.IsZero() {
			t += " · Fim: " + end.Format("2006-01-02 15:04")
		}
		rows = append(rows, row.New(4).Add(text.NewCol(12, t, props.Text{Size: 7})))
	}
	return rows
}

// taxSummaryRows prints the per-rate VAT summary, with exemption codes when present.
func taxSummaryRows(b domain.TaxBreakdown) []core.Row {
	if len(b) == 0 {
		return nil
	}
	rows := []core.Row{
		row.New(5).Add(
			text.NewCol(6, "Resumo de impostos", props.Text{Size: 8, Style: fontstyle.Bold, Top: 2}),
			text.NewCol(3, "Base", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right, Top: 2}),
			text.NewCol(3, "Imposto", props.Text{Size: 8, Style: fontstyle.Bold, Align: align.Right, Top: 2}),
		),
	}
	for _, e := range b {
		label := string(e.Category) + " (" + string(e.Region) + ")"
		if rate, err := domain.GetTaxRate(e.Region, e.Category, e.ExemptionCode); err == nil {
			label = "IVA " + fmtPercent(rate.Value) + " (" + string(e.Region) + ")"
		}
		if e.ExemptionCode != "" {
			label += " · " + string(e.ExemptionCode)
			if e.ExemptionDescription != "" {
				label += " — " + e.ExemptionDescription
			}
		}
		rows = append(rows, row.New(4).Add(
			text.NewCol(6, label, props.Text{Size: 7}),
			text.NewCol(3, fmtEUR(e.Base), props.Text{Size: 7, Align: align.Right}),
			text.NewCol(3, fmtEUR(e.Tax), props.Text{Size: 7, Align: align.Right}),
		))
	}
	return rows
}

// totalLine is one right-aligned label/value row in a totals block.
func totalLine(label, value string, bold bool) core.Row {
	style := fontstyle.Normal
	if bold {
		style = fontstyle.Bold
	}
	return row.New(4).Add(
		col.New(6),
		text.NewCol(3, label, props.Text{Size: 8, Style: style, Align: align.Right}),
		text.NewCol(3, value, props.Text{Size: 8, Style: style, Align: align.Right}),
	)
}

// withholdingLabel names one retention entry on the totals block.
func withholdingLabel(w domain.WithholdingTax) string {
	if w.Description != "" {
		return "Retenção na fonte (" + w.Description + ")"
	}
	return "Retenção na fonte"
}

// totalsRows prints the totals block; withholding (when present) and the
// resulting "Total a pagar" distinction; stamp duty only when nonzero.
func totalsRows(t domain.Totals, wh []domain.WithholdingTax) []core.Row {
	rows := []core.Row{
		totalLine("Total líquido", fmtEUR(t.NetTotal), false),
		totalLine("Total IVA", fmtEUR(t.TaxTotal), false),
	}
	if t.StampDuty != 0 {
		rows = append(rows, totalLine("Imposto do Selo", fmtEUR(t.StampDuty), false))
	}
	rows = append(rows, totalLine("Total", fmtEUR(t.GrossTotal), true))
	for _, w := range wh {
		rows = append(rows, totalLine(withholdingLabel(w), "-"+fmtEUR(w.Amount), false))
	}
	if t.AmountPayable != t.GrossTotal {
		rows = append(rows, totalLine("Total a pagar", fmtEUR(t.AmountPayable), true))
	}
	return rows
}

// frPaymentRows prints the FR (Fatura-Recibo) settlement entries.
func frPaymentRows(ps []domain.FRPayment) []core.Row {
	var rows []core.Row
	for _, p := range ps {
		label := fmt.Sprintf("Pagamento: %s · %s · %s",
			string(p.Mechanism), fmtEUR(p.Amount), fmtDate(p.Date))
		rows = append(rows, row.New(4).Add(text.NewCol(12, label, props.Text{Size: 7})))
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

// currencyRows prints the original-currency note for non-EUR documents.
func currencyRows(c *domain.Currency) []core.Row {
	if c == nil {
		return nil
	}
	label := fmt.Sprintf("Moeda original: %s %s",
		string(c.Code), strings.Replace(c.Amount.Format2DP(), ".", ",", 1))
	return []core.Row{
		row.New(4).Add(text.NewCol(12, label, props.Text{Size: 7})),
	}
}
