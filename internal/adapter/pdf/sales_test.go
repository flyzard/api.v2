package pdf

import (
	"errors"
	"os"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"github.com/johnfercher/maroto/v2/pkg/test"
)

func TestBuildSalesInvoice_FT_Structure(t *testing.T) {
	eng, err := buildSalesInvoice(fixtureFT(t), validMeta())
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
			eng, err := buildSalesInvoice(tc.inv, validMeta())
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
