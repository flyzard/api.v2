package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
)

// WriteDocumentPDFs renders every issued document to outDir/*.pdf — the
// certification dossier needs a PDF for each checklist document. Each document is
// rendered once per app.RequiredVias (Original + Duplicado; transport documents
// also Triplicado + Quadruplicado), all through app.QueryService.RenderPDF.
func WriteDocumentPDFs(c *Ctx, outDir string) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}
	for _, v := range c.store.views() {
		vias, err := app.RequiredVias(v.Type)
		if err != nil {
			log.Fatalf("required vias %s: %v", v.Type, err)
		}
		for _, via := range vias {
			name := pdfName(v, via)
			data, rerr := c.svc.Query.RenderPDF(context.Background(), c.tenant, v.Number, via)
			if rerr != nil {
				log.Fatalf("render %s: %v", name, rerr)
			}
			path := filepath.Join(outDir, name)
			if werr := os.WriteFile(path, data, 0o644); werr != nil {
				log.Fatalf("write %s: %v", path, werr)
			}
			fmt.Printf("PDF written: %s (%d bytes)\n", path, len(data))
		}
	}
}

// pdfName is the outDir filename for one via of a document; recordChecklist
// relies on it so checklist rows always match the rendered files. The smoke uses
// one series per type, so type+seq is unique.
func pdfName(v app.IssuedView, via app.CopyKind) string {
	suffix := map[app.CopyKind]string{
		app.Duplicado:     "-duplicado",
		app.Triplicado:    "-triplicado",
		app.Quadruplicado: "-quadruplicado",
		app.SegundaVia:    "-2avia", // without it a reprint would overwrite the Original file
	}[via]
	return fmt.Sprintf("%s-%d%s.pdf", v.Type, v.Seq, suffix)
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
