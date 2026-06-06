package pdf

import (
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// buildStockMovement assembles the maroto document for GT/GR/GA/GC/GD.
// Prints transport places/times and the AT-assigned code (ATDocCodeID) the
// carrier must present; always "Este documento não serve de fatura".
func buildStockMovement(sm domain.StockMovement, m Meta) (core.Maroto, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	if sm.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}
	id := docIdentity{Type: sm.DocumentType, Number: sm.Number, Date: sm.Date}
	if err := eng.RegisterHeader(headerRows(m, id, sm.Customer)...); err != nil {
		return nil, err
	}
	if sm.Status == domain.StatusCancelled {
		eng.AddRows(cancelledRows(sm.Reason)...)
	}
	if sm.ATDocCodeID != "" {
		eng.AddRows(row.New(5).Add(text.NewCol(12, "Código AT: "+sm.ATDocCodeID,
			props.Text{Size: 9, Style: fontstyle.Bold})))
	}
	eng.AddRows(shippingRows(sm.ShipFrom, sm.ShipTo, &sm.MovementStartTime, sm.MovementEndTime)...)
	eng.AddRows(linesTable(sm.Lines)...)
	eng.AddRows(taxSummaryRows(sm.Totals.Breakdown)...)
	eng.AddRows(totalsRows(sm.Totals, nil)...)
	footer, err := legalFooterRows(sm.ATCUD, sm.QRPayload, sm.Hash, m.CertNumber,
		notInvoiceMention(sm.DocumentType))
	if err != nil {
		return nil, err
	}
	eng.AddRows(footer...)
	return eng, nil
}

// RenderStockMovement renders an issued transport document as PDF bytes.
func RenderStockMovement(sm domain.StockMovement, m Meta) ([]byte, error) {
	return render(buildStockMovement(sm, m))
}
