package pdf

import (
	"errors"
	"slices"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func validMeta() Meta {
	return Meta{
		Seller: Seller{
			Name:       "Empresa Exemplo Lda",
			TaxID:      "555555550",
			Address:    "Rua do Exemplo 1",
			City:       "Lisboa",
			PostalCode: "1000-001",
		},
		CertNumber: "9999",
		Copy:       Original,
	}
}

func TestMetaValidate(t *testing.T) {
	if err := validMeta().validate(); err != nil {
		t.Fatalf("valid meta rejected: %v", err)
	}

	m := validMeta()
	m.Seller.Name = ""
	if err := m.validate(); !errors.Is(err, ErrMissingSellerName) {
		t.Fatalf("want ErrMissingSellerName, got %v", err)
	}

	m = validMeta()
	m.Seller.TaxID = ""
	if err := m.validate(); !errors.Is(err, ErrMissingSellerTaxID) {
		t.Fatalf("want ErrMissingSellerTaxID, got %v", err)
	}

	m = validMeta()
	m.CertNumber = ""
	if err := m.validate(); !errors.Is(err, ErrMissingCertNumber) {
		t.Fatalf("want ErrMissingCertNumber, got %v", err)
	}

	// Issuer's full address is mandatory print content (Portaria 363/2010).
	for _, blank := range []func(*Meta){
		func(m *Meta) { m.Seller.Address = "" },
		func(m *Meta) { m.Seller.PostalCode = "" },
		func(m *Meta) { m.Seller.City = "" },
	} {
		m = validMeta()
		blank(&m)
		if err := m.validate(); !errors.Is(err, ErrMissingSellerAddress) {
			t.Fatalf("want ErrMissingSellerAddress, got %v", err)
		}
	}
}

func TestCopyKindLabel(t *testing.T) {
	cases := map[CopyKind]string{
		Original:      "Original",
		Duplicado:     "Duplicado",
		Triplicado:    "Triplicado",
		Quadruplicado: "Quadruplicado",
		SegundaVia:    "2.ª via",
	}
	for k, want := range cases {
		if got := k.label(); got != want {
			t.Errorf("label(%d) = %q, want %q", k, got, want)
		}
	}
}

func TestRequiredVias(t *testing.T) {
	if got := RequiredVias(domain.GT); !slices.Equal(got, []CopyKind{Original, Duplicado, Triplicado, Quadruplicado}) {
		t.Errorf("transport vias = %v, want Original+Duplicado+Triplicado+Quadruplicado", got)
	}
	for _, dt := range []domain.DocumentType{domain.FT, domain.FS, domain.NE, domain.RC} {
		if got := RequiredVias(dt); !slices.Equal(got, []CopyKind{Original, Duplicado}) {
			t.Errorf("RequiredVias(%s) = %v, want Original+Duplicado", dt, got)
		}
	}
}
