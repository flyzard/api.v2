package pdf

import (
	"github.com/johnfercher/maroto/v2/pkg/core"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// buildSalesInvoice assembles the maroto document for FT/FS/FR/NC/ND.
// Split from RenderSalesInvoice so structure tests can assert the layout
// tree without generating PDF bytes.
func buildSalesInvoice(inv domain.SalesInvoice, m Meta) (core.Maroto, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	if inv.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}
	id := docIdentity{Type: inv.DocumentType, Number: inv.Number, Date: inv.Date, DueDate: inv.PaymentTerms}
	if err := eng.RegisterHeader(headerRows(m, id, inv.Customer)...); err != nil {
		return nil, err
	}
	if inv.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(inv.Reason)...)
	}
	eng.AddRows(linesTable(inv.Lines)...)
	eng.AddRows(referencesRows(inv.Lines)...)
	eng.AddRows(shippingRows(inv.ShipFrom, inv.ShipTo, inv.MovementStartTime, inv.MovementEndTime)...)
	eng.AddRows(taxSummaryRows(inv.Totals.Breakdown)...)
	eng.AddRows(totalsRows(inv.Totals, inv.WithholdingTax)...)
	eng.AddRows(frPaymentRows(inv.Payments)...)
	eng.AddRows(currencyRows(inv.Currency)...)
	eng.AddRows(regimeRows(inv.SpecialRegimes)...)
	footer, err := legalFooterRows(inv.ATCUD, inv.QRPayload, inv.Hash, m.CertNumber,
		notInvoiceMention(inv.DocumentType))
	if err != nil {
		return nil, err
	}
	eng.AddRows(footer...)
	return eng, nil
}

// RenderSalesInvoice renders an issued FT/FS/FR/NC/ND as PDF bytes.
func RenderSalesInvoice(inv domain.SalesInvoice, m Meta) ([]byte, error) {
	return render(buildSalesInvoice(inv, m))
}
