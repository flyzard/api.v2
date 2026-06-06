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
// footerATCUD: see legalFooterRows.
func buildStockMovement(sm domain.StockMovement, m Meta, footerATCUD bool) (core.Maroto, error) {
	if sm.QRPayload == "" {
		return nil, ErrMissingQRPayload
	}
	id := docIdentity{Type: sm.DocumentType, Number: sm.Number, Date: sm.Date}
	eng, err := newDocEngine(m, id, sm.Customer, sm.ATCUD, sm.Hash, footerATCUD)
	if err != nil {
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
	eng.AddRows(summaryAndTotalsRows(sm.Totals.Breakdown, sm.Lines,
		salesTotals(sm.Totals, nil, sm.Lines))...)
	qr, err := qrRows(sm.ATCUD, sm.QRPayload)
	if err != nil {
		return nil, err
	}
	eng.AddRows(qr...)
	return eng, nil
}

// RenderStockMovement renders an issued transport document as PDF bytes.
func RenderStockMovement(sm domain.StockMovement, m Meta) ([]byte, error) {
	return renderAdaptive(func(footerATCUD bool) (core.Maroto, error) {
		return buildStockMovement(sm, m, footerATCUD)
	})
}
