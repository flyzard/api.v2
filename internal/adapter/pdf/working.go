package pdf

import (
	"github.com/johnfercher/maroto/v2/pkg/core"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// buildWorkDocument assembles the maroto document for OR/PF/NE/CM/FC/FO/OU.
// Work documents always print "Este documento não serve de fatura".
func buildWorkDocument(wd domain.WorkDocument, m Meta) (core.Maroto, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	if wd.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}
	id := docIdentity{Type: wd.DocumentType, Number: wd.Number, Date: wd.Date, DueDate: wd.PaymentTerms}
	if err := eng.RegisterHeader(headerRows(m, id, wd.Customer)...); err != nil {
		return nil, err
	}
	if wd.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(wd.Reason)...)
	}
	eng.AddRows(linesTable(wd.Lines)...)
	eng.AddRows(taxSummaryRows(wd.Totals.Breakdown)...)
	eng.AddRows(totalsRows(wd.Totals, nil)...)
	footer, err := legalFooterRows(wd.ATCUD, wd.QRPayload, wd.Hash, m.CertNumber,
		notInvoiceMention(wd.DocumentType))
	if err != nil {
		return nil, err
	}
	eng.AddRows(footer...)
	return eng, nil
}

// RenderWorkDocument renders an issued work document as PDF bytes.
func RenderWorkDocument(wd domain.WorkDocument, m Meta) ([]byte, error) {
	return render(buildWorkDocument(wd, m))
}
