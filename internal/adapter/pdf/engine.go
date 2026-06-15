package pdf

import (
	"bytes"
	_ "embed"
	"errors"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/johnfercher/maroto/v2/pkg/repository"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

//go:embed fonts/LiberationSans-Regular.ttf
var fontRegular []byte

//go:embed fonts/LiberationSans-Bold.ttf
var fontBold []byte

// fontFamily is the embedded UTF-8 font (Portuguese diacritics).
const fontFamily = "liberation"

// newEngine builds a configured A4 maroto instance: embedded fonts, 10 mm
// margins (maroto defaults), automatic "Página n de N" footer.
func newEngine() (core.Maroto, error) {
	fonts, err := repository.New().
		AddUTF8FontFromBytes(fontFamily, fontstyle.Normal, fontRegular).
		AddUTF8FontFromBytes(fontFamily, fontstyle.Bold, fontBold).
		Load()
	if err != nil {
		return nil, err
	}
	cfg := config.NewBuilder().
		WithCustomFonts(fonts).
		WithDefaultFont(&props.Font{Family: fontFamily, Size: 9}).
		WithPageNumber(props.PageNumber{
			Pattern: "Página {current} de {total}",
			Place:   props.RightBottom,
			Size:    7,
		}).
		Build()
	return maroto.New(cfg), nil
}

// newDocEngine is the shared head of every build*: validates Meta, builds the
// engine and registers the per-page header and legal footer. Registration
// order (header, then footer, before any content rows) lives only here.
// The QR block is NOT registered here — build* appends qrRows as the last
// content rows (see qrRows for why it must flow with the content).
// footerATCUD: see legalFooterRows.
func newDocEngine(m Meta, id docIdentity, cust domain.Customer,
	atcud domain.ATCUD, hash domain.Hash, footerATCUD bool) (core.Maroto, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}
	if err := eng.RegisterHeader(headerRows(m, id, cust)...); err != nil {
		return nil, err
	}
	footer := legalFooterRows(atcud, hash, m.CertNumber, notInvoiceMention(id.Type), footerATCUD)
	if err := eng.RegisterFooter(footer...); err != nil {
		return nil, err
	}
	return eng, nil
}

// render turns a build* result into PDF bytes; shared tail of every Render*.
func render(eng core.Maroto, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	doc, err := eng.Generate()
	if err != nil {
		return nil, err
	}
	return doc.GetBytes(), nil
}

// renderAdaptive renders documents that carry a QR block. The ATCUD must sit
// immediately above the QR AND appear on every page (Despacho 412/2020-XXII);
// on a single-page document the qrRows copy alone satisfies both, so the
// footer omits it and the ATCUD prints exactly once. Pagination is only known
// after generating, so: render without the footer ATCUD, and only when the
// document spans more than one page re-render with it (QR-less pages need
// their own copy — the QR's page then shows it twice, which the adjacency +
// every-page rules together require).
func renderAdaptive(build func(footerATCUD bool) (core.Maroto, error)) ([]byte, error) {
	b, err := render(build(false))
	if err != nil {
		return nil, err
	}
	n := pageCount(b)
	if n == 0 {
		// The pages tree was not found — pagination is unknown, so the
		// ATCUD-on-every-page rule (Despacho 412/2020-XXII) cannot be
		// guaranteed. Refusing beats silently printing a non-compliant PDF.
		return nil, errors.New("pdf: page count not found in generated PDF")
	}
	if n == 1 {
		return b, nil
	}
	return render(build(true))
}

// pageCount reads the page total from the PDF's pages-tree object
// ("/Type /Pages … /Count N", written uncompressed by gofpdf) — maroto's
// Document API does not expose it. 0 means the object was not found.
func pageCount(pdf []byte) int {
	i := bytes.Index(pdf, []byte("/Type /Pages"))
	if i < 0 {
		return 0
	}
	j := bytes.Index(pdf[i:], []byte("/Count "))
	if j < 0 {
		return 0
	}
	n := 0
	for _, c := range pdf[i+j+len("/Count "):] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
