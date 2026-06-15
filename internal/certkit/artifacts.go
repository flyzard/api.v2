package certkit

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/pdf"
	"github.com/flyzard/invoicing.v2/internal/adapter/saft"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// WriteSAFT projects every recorded document into a single SAF-T XML file under
// outDir. The period spans the previous month through the current one so the
// export carries documents from two different months (cert letter).
func WriteSAFT(c *Ctx, sw config.SoftwareIdentity, now time.Time, outDir string) {
	loc := now.Location()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	start := firstOfMonth.AddDate(0, -1, 0)
	end := firstOfMonth.AddDate(0, 1, -1)

	hdr := saft.Header{
		Issuer: c.f.Issuer,
		Software: saft.SoftwareIdentity{
			ProducerTaxID:     sw.ProducerTaxID,
			CertificateNumber: sw.CertificateNumber,
			ProductID:         sw.ProductID(),
			Version:           sw.Version,
		},
		Start:     start,
		End:       end,
		CreatedAt: now,
	}
	out, err := saft.Export(hdr,
		c.store.snapshotSales(),
		c.store.snapshotStock(),
		c.store.snapshotWork(),
		c.store.snapshotPayments(),
	)
	if err != nil {
		log.Fatalf("saft export: %v", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}
	path := filepath.Join(outDir, fmt.Sprintf("SAFT-DEMO-%s-%s.xml", start.Format("2006-01"), end.Format("2006-01")))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	fmt.Printf("\nSAF-T written: %s (%d bytes)\n", path, len(out))
}

// WriteDocumentPDFs renders every issued document to outDir/*.pdf — the
// certification dossier needs a PDF for each checklist document. Each document
// is rendered once per pdf.RequiredVias (Original + Duplicado; transport
// documents also Triplicado).
func WriteDocumentPDFs(c *Ctx, sw config.SoftwareIdentity, outDir string) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	meta := pdf.Meta{
		Seller: pdf.Seller{
			Name:       c.f.Issuer.Name,
			TaxID:      string(c.f.Issuer.NIF),
			Address:    c.f.Issuer.Address.AddressDetail,
			City:       c.f.Issuer.Address.City,
			PostalCode: c.f.Issuer.Address.PostalCode,
		},
		CertNumber: sw.CertificateNumber,
	}

	// Every document in each store family, lowest sequence number first
	// (deterministic across runs).
	renderFamily(c.store.snapshotSales(), meta, outDir,
		func(d domain.SalesInvoice) domain.DocNumber { return d.Number },
		pdf.RenderSalesInvoice)
	renderFamily(c.store.snapshotStock(), meta, outDir,
		func(d domain.StockMovement) domain.DocNumber { return d.Number },
		pdf.RenderStockMovement)
	renderFamily(c.store.snapshotWork(), meta, outDir,
		func(d domain.WorkDocument) domain.DocNumber { return d.Number },
		pdf.RenderWorkDocument)
	renderFamily(c.store.snapshotPayments(), meta, outDir,
		func(d domain.Payment) domain.DocNumber { return d.Number },
		pdf.RenderPayment)
}

// pdfName is the outDir filename for one via of a document; recordChecklist
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

// renderFamily writes outDir/<pdfName>.pdf for every required via of every
// document in docs.
func renderFamily[T any](docs []T, meta pdf.Meta, outDir string,
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
			path := filepath.Join(outDir, name)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				log.Fatalf("write %s: %v", path, err)
			}
			fmt.Printf("PDF written: %s (%d bytes)\n", path, len(data))
		}
	}
}

// MustLisbon loads Europe/Lisbon or dies — every certification timestamp is in
// that zone.
func MustLisbon() *time.Location {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("cannot load Europe/Lisbon: " + err.Error())
	}
	return loc
}
