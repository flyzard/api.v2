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

// legalFooterRows builds the text-only regulatory footer, registered as the
// maroto footer so it is pinned to the bottom of every page:
//   - ATCUD, only when includeATCUD — it must appear on all pages (Despacho
//     412/2020-XXII), but on a single-page document the qrRows copy already
//     satisfies that, and the certifier prefers it printed once. Receipts
//     (no QR block) and multi-page documents include it here; renderAdaptive
//     owns the single/multi-page decision.
//   - 4 signature chars + "Processado por programa certificado n.º <c>/AT"
//     (Portaria 363/2010 Art. 6; hash chars omitted when hash is empty — payments)
//   - per-family mention ("Este documento não serve de fatura") when set
//
// ATCUD and the signature line print at 10/9 pt in plain black — 2026
// certification feedback flagged smaller sizes as illegible on paper
// (DL 28/2019 Art. 8 legibility).
//
// The QR deliberately does NOT live here: 2026 certification feedback — a QR
// at the bottom of the document falls inside most printers' unprintable
// margin, gets cut and becomes invalid. It flows with the content instead
// (qrRows); only safe-to-trim text sits near the paper edge.
func legalFooterRows(atcud domain.ATCUD, hash domain.Hash, cert, mention string, includeATCUD bool) []core.Row {
	sig := "Processado por programa certificado n.º " + cert + "/AT"
	if hc := hash.FourChars(); hc != "" { // canonical domain extraction (positions 1/11/21/31)
		sig = hc + " - " + sig
	}

	rows := []core.Row{divider(2)}
	if includeATCUD {
		rows = append(rows, row.New(5).Add(text.NewCol(12, "ATCUD: "+string(atcud),
			props.Text{Size: 10, Style: fontstyle.Bold, Top: 0.5})))
	}
	rows = append(rows, row.New(5).Add(text.NewCol(12, sig, props.Text{Size: 9, Top: 0.5})))
	if mention != "" {
		rows = append(rows, row.New(4).Add(text.NewCol(12, mention,
			props.Text{Size: 7, Style: fontstyle.Bold})))
	}
	return rows
}

// qrRows is the QR block appended as the LAST content rows of the document
// (payments excepted — no QR): the ATCUD immediately above the QR (Despacho
// 412/2020-XXII adjacency), then the symbol itself — ≥30×30 mm: 3-grid col
// (47.5 mm) × 32 mm row, Percent 100 → 32 mm square. The PNG comes from
// internal/adapter/qrimage, which enforces ECC=M and the AT symbol-version
// ≥ 9 floor and keeps the 4-module quiet zone.
//
// Living in the content flow (not the footer) keeps the QR clear of the
// paper edge, and maroto never splits a row across pages — the 32 mm QR row
// moves whole to the next page when it doesn't fit, so it cannot be cut.
// The ATCUD repeats in the page footer; on the QR's page it prints twice,
// which the adjacency + every-page rules together require for multi-page
// documents.
func qrRows(atcud domain.ATCUD, qrPayload string) ([]core.Row, error) {
	png, err := qrimage.PNG(qrPayload, qrSizePx)
	if err != nil {
		return nil, err
	}
	return []core.Row{
		row.New(4),
		row.New(5).Add(
			text.NewCol(3, "ATCUD: "+string(atcud),
				props.Text{Size: 10, Style: fontstyle.Bold, Top: 0.5}),
			col.New(9),
		),
		row.New(32).Add(
			image.NewFromBytesCol(3, png, extension.Png, props.Rect{Percent: 100}),
			col.New(9),
		),
	}, nil
}

// notInvoiceMention returns the Portaria 363/2010 mention for documents that
// are not invoices nor invoice-correcting documents. Receipts (RC/RG) get the
// mention deliberately: a receipt settles an invoice but does not replace one,
// and certified-software practice is to print it there too.
func notInvoiceMention(dt domain.DocumentType) string {
	if dt.IsSales() {
		return ""
	}
	return "Este documento não serve de fatura"
}
