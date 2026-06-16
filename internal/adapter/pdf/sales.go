package pdf

import (
	"github.com/johnfercher/maroto/v2/pkg/core"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// buildSalesInvoice assembles the maroto document for FT/FS/FR/NC/ND.
// Split from RenderSalesInvoice so structure tests can assert the layout
// tree without generating PDF bytes. footerATCUD: see legalFooterRows.
func buildSalesInvoice(inv domain.SalesInvoice, m Meta, footerATCUD bool) (core.Maroto, error) {
	if inv.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	id := docIdentity{Type: inv.DocumentType, Number: inv.Number, Date: inv.Date, DueDate: inv.PaymentTerms}
	eng, err := newDocEngine(m, id, inv.Customer, inv.ATCUD, inv.Hash, footerATCUD, inv.Status == domain.StatusCancelled)
	if err != nil {
		return nil, err
	}
	if inv.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(inv.Reason)...)
	}
	eng.AddRows(linesTable(inv.Lines)...)
	eng.AddRows(referencesRows(inv.Lines)...)
	eng.AddRows(shippingRows(inv.ShipFrom, inv.ShipTo, inv.MovementStartTime, inv.MovementEndTime)...)
	eng.AddRows(summaryAndTotalsRows(inv.Totals.Breakdown, inv.Lines,
		salesTotals(inv.Totals, inv.WithholdingTax, inv.Lines))...)
	eng.AddRows(frPaymentRows(inv.Payments)...)
	eng.AddRows(currencyRows(inv.Currency)...)
	eng.AddRows(regimeRows(inv.SpecialRegimes)...)
	qr, err := qrRows(inv.ATCUD, inv.QRPayload)
	if err != nil {
		return nil, err
	}
	eng.AddRows(qr...)
	return eng, nil
}

// RenderSalesInvoice renders an issued FT/FS/FR/NC/ND as PDF bytes.
func RenderSalesInvoice(inv domain.SalesInvoice, m Meta) ([]byte, error) {
	return renderAdaptive(func(footerATCUD bool) (core.Maroto, error) {
		return buildSalesInvoice(inv, m, footerATCUD)
	})
}
