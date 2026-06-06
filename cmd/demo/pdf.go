package main

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/flyzard/invoicing.v2/internal/adapter/pdf"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// writeDocumentPDFs renders every issued document to out/*.pdf — the
// certification dossier needs a PDF for each checklist document, and the
// demo issues at least one document of every type the app supports. Each
// document is rendered once per pdf.RequiredVias (Original + Duplicado;
// transport documents also Triplicado).
func writeDocumentPDFs(c *ctx, f *fixtures, sw config.SoftwareIdentity) {
	if err := os.MkdirAll("out", 0o755); err != nil {
		log.Fatalf("mkdir out: %v", err)
	}

	meta := pdf.Meta{
		Seller: pdf.Seller{
			Name:       f.Issuer.Name,
			TaxID:      string(f.Issuer.NIF),
			Address:    f.Issuer.Address.AddressDetail,
			City:       f.Issuer.Address.City,
			PostalCode: f.Issuer.Address.PostalCode,
		},
		CertNumber: sw.CertificateNumber,
	}

	// Every document in each store family, lowest sequence number first
	// (deterministic across runs).
	renderFamily(c.store.snapshotSales(), meta,
		func(d domain.SalesInvoice) domain.DocNumber { return d.Number },
		pdf.RenderSalesInvoice)
	renderFamily(c.store.snapshotStock(), meta,
		func(d domain.StockMovement) domain.DocNumber { return d.Number },
		pdf.RenderStockMovement)
	renderFamily(c.store.snapshotWork(), meta,
		func(d domain.WorkDocument) domain.DocNumber { return d.Number },
		pdf.RenderWorkDocument)
	renderFamily(c.store.snapshotPayments(), meta,
		func(d domain.Payment) domain.DocNumber { return d.Number },
		pdf.RenderPayment)
}

// pdfName is the out/ filename for one via of a document; recordChecklist
// relies on it so checklist rows always match the rendered files. The demo
// uses one series per type, so type+seq is unique.
func pdfName(n domain.DocNumber, via pdf.CopyKind) string {
	suffix := map[pdf.CopyKind]string{
		pdf.Duplicado:  "-duplicado",
		pdf.Triplicado: "-triplicado",
		pdf.SegundaVia: "-2avia", // without it a reprint would overwrite the Original file
	}[via]
	return fmt.Sprintf("%s-%d%s.pdf", string(n.Type), n.Seq, suffix)
}

// renderFamily writes out/<pdfName>.pdf for every required via of every
// document in docs.
func renderFamily[T any](docs []T, meta pdf.Meta,
	number func(T) domain.DocNumber, render func(T, pdf.Meta) ([]byte, error)) {
	// Tie-break on Series: same doc type can live in two series with equal
	// Seq, and slices.SortFunc is unstable on ties.
	slices.SortFunc(docs, func(a, b T) int {
		na, nb := number(a), number(b)
		return cmp.Or(cmp.Compare(na.Seq, nb.Seq), cmp.Compare(na.Series, nb.Series))
	})
	for _, d := range docs {
		for _, via := range pdf.RequiredVias(number(d).Type) {
			meta.Copy = via
			name := pdfName(number(d), via)
			data, err := render(d, meta)
			if err != nil {
				log.Fatalf("render %s: %v", name, err)
			}
			path := filepath.Join("out", name)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				log.Fatalf("write %s: %v", path, err)
			}
			fmt.Printf("PDF written: %s (%d bytes)\n", path, len(data))
		}
	}
}
