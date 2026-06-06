package pdf

import (
	"github.com/johnfercher/maroto/v2/pkg/core"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// buildWorkDocument assembles the maroto document for OR/PF/NE/CM/FC/FO/OU.
// Work documents always print "Este documento não serve de fatura".
// footerATCUD: see legalFooterRows.
func buildWorkDocument(wd domain.WorkDocument, m Meta, footerATCUD bool) (core.Maroto, error) {
	if wd.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	id := docIdentity{Type: wd.DocumentType, Number: wd.Number, Date: wd.Date, DueDate: wd.PaymentTerms}
	eng, err := newDocEngine(m, id, wd.Customer, wd.ATCUD, wd.Hash, footerATCUD)
	if err != nil {
		return nil, err
	}
	if wd.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(wd.Reason)...)
	}
	eng.AddRows(linesTable(wd.Lines)...)
	eng.AddRows(summaryAndTotalsRows(wd.Totals.Breakdown, wd.Lines,
		salesTotals(wd.Totals, nil, wd.Lines))...)
	qr, err := qrRows(wd.ATCUD, wd.QRPayload)
	if err != nil {
		return nil, err
	}
	eng.AddRows(qr...)
	return eng, nil
}

// RenderWorkDocument renders an issued work document as PDF bytes.
func RenderWorkDocument(wd domain.WorkDocument, m Meta) ([]byte, error) {
	return renderAdaptive(func(footerATCUD bool) (core.Maroto, error) {
		return buildWorkDocument(wd, m, footerATCUD)
	})
}
