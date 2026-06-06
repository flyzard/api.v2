package pdf

import (
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/flyzard/invoicing.v2/internal/adapter/qrimage"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// qrSizePx is the raster edge for the embedded QR PNG: 32 mm at ~300 dpi.
// qrimage may return larger when the symbol needs it; module integrity wins.
const qrSizePx = 384

// legalFooterRows builds the regulatory footer block:
//   - ATCUD immediately above the QR (Despacho 412/2020-XXII)
//   - QR ≥30×30 mm: 3-grid col (47.5 mm) × 32 mm row, Percent 100 → 32 mm
//     square. The PNG comes from internal/adapter/qrimage, which enforces
//     ECC=M and the AT symbol-version ≥ 9 floor and keeps the 4-module
//     quiet zone; the empty neighbouring columns add print clearance.
//   - 4 signature chars + "Processado por programa certificado n.º <c>/AT"
//     (omitted hash chars when hash is empty — payments)
//   - per-family mention ("Este documento não serve de fatura") when set
//
// qrPayload == "" (payments) skips the ATCUD-above-QR + QR rows; the ATCUD
// then prints as a plain line.
func legalFooterRows(atcud domain.ATCUD, qrPayload string, hash domain.Hash, cert, mention string) ([]core.Row, error) {
	rows := []core.Row{
		row.New(4).Add(text.NewCol(12, "ATCUD: "+string(atcud), props.Text{Size: 8, Top: 2})),
	}
	if qrPayload != "" {
		png, err := qrimage.PNG(qrPayload, qrSizePx)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row.New(32).Add(
			image.NewFromBytesCol(3, png, extension.Png, props.Rect{Percent: 100}),
			col.New(9),
		))
	}

	sig := "Processado por programa certificado n.º " + cert + "/AT"
	if hc := hash.FourChars(); hc != "" { // canonical domain extraction (positions 1/11/21/31)
		sig = hc + " - " + sig
	}
	rows = append(rows, row.New(4).Add(text.NewCol(12, sig, props.Text{Size: 7})))

	if mention != "" {
		rows = append(rows, row.New(4).Add(text.NewCol(12, mention,
			props.Text{Size: 7, Style: fontstyle.Bold})))
	}
	return rows, nil
}

// notInvoiceMention returns the Portaria 363/2010 mention for documents that
// are not invoices nor invoice-correcting documents.
func notInvoiceMention(dt domain.DocumentType) string {
	if dt.IsSales() {
		return ""
	}
	return "Este documento não serve de fatura"
}
