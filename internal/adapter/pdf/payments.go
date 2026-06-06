package pdf

import (
	"fmt"

	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// buildPayment assembles the maroto document for RC/RG receipts.
// Receipts carry no Hash and no QRPayload (domain decision — see spec):
// the legal footer prints ATCUD + the certified-software mention only.
func buildPayment(p domain.Payment, m Meta) (core.Maroto, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}
	id := docIdentity{Type: p.Type, Number: p.Number, Date: p.TransactionDate}
	if err := eng.RegisterHeader(headerRows(m, id, p.Customer)...); err != nil {
		return nil, err
	}
	if p.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(p.Reason)...)
	}
	eng.AddRows(paymentLinesTable(p.Lines)...)
	for _, meth := range p.Methods {
		eng.AddRows(row.New(4).Add(text.NewCol(12,
			fmt.Sprintf("Meio de pagamento: %s · %s · %s",
				string(meth.Mechanism), fmtEUR(meth.Amount), fmtDate(meth.Date)),
			props.Text{Size: 7})))
	}
	eng.AddRows(paymentTotalsRows(p)...)
	eng.AddRows(currencyRows(p.Currency)...)
	footer, err := legalFooterRows(p.ATCUD, "", "", m.CertNumber,
		notInvoiceMention(p.Type))
	if err != nil {
		return nil, err
	}
	eng.AddRows(footer...)
	return eng, nil
}

// paymentLinesTable lists the settled source documents.
func paymentLinesTable(lines []domain.PaymentLine) []core.Row {
	rows := []core.Row{
		row.New(6).Add(
			text.NewCol(6, "Documento", tableHdr),
			text.NewCol(3, "Data", tableRightHdr),
			text.NewCol(3, "Valor", tableRightHdr),
		),
	}
	for _, l := range lines {
		amount := ""
		if l.Movement != nil {
			amount = fmtEUR(l.Movement.Amount())
		}
		for _, sd := range l.SourceDocuments {
			rows = append(rows, row.New(5).Add(
				text.NewCol(6, sd.OriginatingON, tableCell),
				text.NewCol(3, fmtDate(sd.InvoiceDate), tableRightCell),
				text.NewCol(3, amount, tableRightCell),
			))
			amount = "" // print the line amount once when it spans several sources
		}
	}
	return rows
}

// paymentTotalsRows prints the receipt totals; withholding reduces the
// derived "Total recebido" (PaymentTotals carries no AmountPayable).
func paymentTotalsRows(p domain.Payment) []core.Row {
	rows := []core.Row{
		totalLine("Total líquido", fmtEUR(p.NetTotal), false),
		totalLine("Total IVA", fmtEUR(p.TaxPayable), false),
		totalLine("Total", fmtEUR(p.GrossTotal), true),
	}
	var withheld domain.Money
	for _, w := range p.WithholdingTax {
		rows = append(rows, totalLine(withholdingLabel(w), "-"+fmtEUR(w.Amount), false))
		withheld += w.Amount
	}
	if withheld != 0 {
		rows = append(rows, totalLine("Total recebido", fmtEUR(p.GrossTotal.Sub(withheld)), true))
	}
	return rows
}

// RenderPayment renders an issued RC/RG receipt as PDF bytes.
func RenderPayment(p domain.Payment, m Meta) ([]byte, error) {
	return render(buildPayment(p, m))
}
