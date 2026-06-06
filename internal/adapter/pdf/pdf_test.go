package pdf

import (
	"errors"
	"testing"
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
}

func TestCopyKindLabel(t *testing.T) {
	cases := map[CopyKind]string{
		Original:   "Original",
		Duplicado:  "Duplicado",
		SegundaVia: "2.ª via",
	}
	for k, want := range cases {
		if got := k.label(); got != want {
			t.Errorf("label(%d) = %q, want %q", k, got, want)
		}
	}
}
