package pdf

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"github.com/johnfercher/maroto/v2/pkg/test"
)

func TestBuildSalesInvoice_FT_Structure(t *testing.T) {
	eng, err := buildSalesInvoice(fixtureFT(t), validMeta(), false)
	if err != nil {
		t.Fatal(err)
	}
	test.New(t).Assert(eng.GetStructure()).Equals("ft_basic.json")
}

func TestRenderSalesInvoice_ProducesPDF(t *testing.T) {
	b, err := RenderSalesInvoice(fixtureFT(t), validMeta())
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 4 || string(b[:4]) != "%PDF" {
		t.Fatalf("output does not start with %%PDF, got %d bytes", len(b))
	}
	// Single-page document: renderAdaptive must settle on the first pass.
	if got := pageCount(b); got != 1 {
		t.Fatalf("pageCount = %d, want 1", got)
	}
}

// TestRenderSalesInvoice_MultiPage exercises the renderAdaptive second pass:
// enough lines to overflow one page must yield a multi-page PDF (whose footer
// then carries the ATCUD on every page — Despacho 412/2020-XXII).
func TestRenderSalesInvoice_MultiPage(t *testing.T) {
	inv := fixtureFT(t)
	for i := 2; i <= 60; i++ {
		l := fixtureLine(t)
		l.LineNumber = i
		inv.Lines = append(inv.Lines, l)
	}
	b, err := RenderSalesInvoice(inv, validMeta())
	if err != nil {
		t.Fatal(err)
	}
	if got := pageCount(b); got < 2 {
		t.Fatalf("pageCount = %d, want >= 2", got)
	}
}

func TestRenderSalesInvoice_InvalidMeta(t *testing.T) {
	m := validMeta()
	m.CertNumber = ""
	if _, err := RenderSalesInvoice(fixtureFT(t), m); !errors.Is(err, ErrMissingCertNumber) {
		t.Fatalf("want ErrMissingCertNumber, got %v", err)
	}
}

func TestRenderSalesInvoice_MissingQR(t *testing.T) {
	inv := fixtureFT(t)
	inv.QRPayload = ""
	if _, err := RenderSalesInvoice(inv, validMeta()); !errors.Is(err, ErrMissingQRPayload) {
		t.Fatalf("want ErrMissingQRPayload, got %v", err)
	}
}

func TestSalesTotals_GlobalDiscount(t *testing.T) {
	inv := fixtureFT(t)
	hasDiscount := func(entries []totalEntry) bool {
		for _, e := range entries {
			if strings.Contains(e.label, "Desconto global") {
				return true
			}
		}
		return false
	}
	if hasDiscount(salesTotals(inv.Totals, nil, inv.Lines)) {
		t.Error("Desconto global entry emitted for a document without a global discount")
	}

	inv.Lines[0].GlobalDiscountShare = mustMoney(t, 3)
	if !hasDiscount(salesTotals(inv.Totals, nil, inv.Lines)) {
		t.Error("Desconto global entry missing")
	}
	// The entry must render through buildSalesInvoice without error too.
	if _, err := buildSalesInvoice(inv, validMeta(), false); err != nil {
		t.Fatalf("buildSalesInvoice with global discount: %v", err)
	}
}

func TestBuildSalesInvoice_Variants(t *testing.T) {
	cases := []struct {
		name   string
		inv    domain.SalesInvoice
		golden string
	}{
		{"nc_references", fixtureNC(t), "nc_references.json"},
		{"ft_cancelled", fixtureFTCancelled(t), "ft_cancelled.json"},
		{"ft_withholding", fixtureFTWithholding(t), "ft_withholding.json"},
		{"fr_payments", fixtureFR(t), "fr_payments.json"},
		{"fs_anonymous", fixtureFSAnonymous(t), "fs_anonymous.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, err := buildSalesInvoice(tc.inv, validMeta(), false)
			if err != nil {
				t.Fatal(err)
			}
			test.New(t).Assert(eng.GetStructure()).Equals(tc.golden)
		})
	}
}

func TestWriteSampleForEyeball(t *testing.T) {
	t.Skip("manual: remove skip to regenerate sample")
	b, err := RenderSalesInvoice(fixtureFT(t), validMeta())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("../../../out", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("../../../out/sample-ft.pdf", b, 0o644); err != nil {
		t.Fatal(err)
	}
}
