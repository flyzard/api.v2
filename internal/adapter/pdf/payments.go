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
// Receipts carry no Hash but do carry a QRPayload (AT C1 ruling); the QR block
// is appended as the last content rows. footerATCUD: see legalFooterRows.
func buildPayment(p domain.Payment, m Meta, footerATCUD bool) (core.Maroto, error) {
	if p.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	id := docIdentity{Type: p.Type, Number: p.Number, Date: p.TransactionDate}
	eng, err := newDocEngine(m, id, p.Customer, p.ATCUD, "", footerATCUD, p.Status == domain.StatusCancelled)
	if err != nil {
		return nil, err
	}
	if p.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(p.Reason)...)
	}
	eng.AddRows(paymentLinesTable(p.Lines)...)
	for _, meth := range p.Methods {
		eng.AddRows(row.New(4).Add(text.NewCol(12,
			fmt.Sprintf("Meio de pagamento: %s · %s · %s",
				mechanismLabel(meth.Mechanism), fmtEUR(meth.Amount), fmtDate(meth.Date)),
			props.Text{Size: 7, Color: gray})))
	}
	eng.AddRows(summaryAndTotalsRows(nil, nil, paymentTotals(p))...)
	eng.AddRows(currencyRows(p.Currency)...)
	qr, err := qrRows(p.ATCUD, p.QRPayload)
	if err != nil {
		return nil, err
	}
	eng.AddRows(qr...)
	return eng, nil
}

// paymentLinesTable lists the settled source documents.
func paymentLinesTable(lines []domain.PaymentLine) []core.Row {
	rows := []core.Row{
		row.New(6.5).Add(
			text.NewCol(6, "Documento", tableHdr),
			text.NewCol(3, "Data", tableRightHdr),
			text.NewCol(3, "Valor", tableRightHdr),
		).WithStyle(tableHdrStyle),
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

// paymentTotals lists the receipt totals; withholding reduces the derived
// "Total recebido" (PaymentTotals carries no AmountPayable). Zero-amount
// retention entries are skipped — a "-0,00 €" row says nothing.
func paymentTotals(p domain.Payment) []totalEntry {
	entries := []totalEntry{
		{"Total líquido", fmtEUR(p.NetTotal), false},
		{"Total IVA", fmtEUR(p.TaxPayable), false},
		{"Total", fmtEUR(p.GrossTotal), true},
	}
	var withheld domain.Money
	for _, w := range p.WithholdingTax {
		if w.Amount == 0 {
			continue
		}
		entries = append(entries, totalEntry{withholdingLabel(w), "-" + fmtEUR(w.Amount), false})
		withheld += w.Amount
	}
	if withheld != 0 {
		entries = append(entries, totalEntry{"Total recebido", fmtEUR(p.GrossTotal.Sub(withheld)), true})
	}
	return entries
}

// RenderPayment renders an issued RC/RG receipt as PDF bytes.
func RenderPayment(p domain.Payment, m Meta) ([]byte, error) {
	return renderAdaptive(func(footerATCUD bool) (core.Maroto, error) {
		return buildPayment(p, m, footerATCUD)
	})
}
