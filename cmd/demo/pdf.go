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

// writeDocumentPDFs renders a representative sample of the demo's issued
// documents to out/*.pdf for visual inspection.
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
		Copy:       pdf.Original,
	}

	// One PDF per document type present in each store family, lowest
	// sequence number first (deterministic across runs).
	sampleFamily(c.store.snapshotSales(),
		func(d domain.SalesInvoice) domain.DocNumber { return d.Number },
		func(d domain.SalesInvoice) ([]byte, error) { return pdf.RenderSalesInvoice(d, meta) })
	sampleFamily(c.store.snapshotStock(),
		func(d domain.StockMovement) domain.DocNumber { return d.Number },
		func(d domain.StockMovement) ([]byte, error) { return pdf.RenderStockMovement(d, meta) })
	sampleFamily(c.store.snapshotWork(),
		func(d domain.WorkDocument) domain.DocNumber { return d.Number },
		func(d domain.WorkDocument) ([]byte, error) { return pdf.RenderWorkDocument(d, meta) })
	sampleFamily(c.store.snapshotPayments(),
		func(d domain.Payment) domain.DocNumber { return d.Number },
		func(d domain.Payment) ([]byte, error) { return pdf.RenderPayment(d, meta) })
}

// sampleFamily writes out/<TYPE>-<seq>.pdf for the first (lowest-seq)
// document of each type in docs.
func sampleFamily[T any](docs []T, number func(T) domain.DocNumber, render func(T) ([]byte, error)) {
	// Tie-break on Series: same doc type can live in two series with equal
	// Seq, and slices.SortFunc is unstable on ties.
	slices.SortFunc(docs, func(a, b T) int {
		na, nb := number(a), number(b)
		return cmp.Or(cmp.Compare(na.Seq, nb.Seq), cmp.Compare(na.Series, nb.Series))
	})
	sampled := map[domain.DocumentType]bool{}
	for _, d := range docs {
		n := number(d)
		if sampled[n.Type] {
			continue
		}
		sampled[n.Type] = true
		name := fmt.Sprintf("%s-%d.pdf", string(n.Type), n.Seq)
		data, err := render(d)
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
