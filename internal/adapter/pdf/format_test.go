package pdf

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func TestFmtEUR(t *testing.T) {
	m, err := domain.NewMoney(1234.56)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmtEUR(m); got != "1234,56 €" {
		t.Fatalf("fmtEUR = %q", got)
	}
}

func TestFmtPercent(t *testing.T) {
	p, err := domain.NewPercent(23)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmtPercent(p); got != "23,00%" {
		t.Fatalf("fmtPercent = %q", got)
	}
}

func TestDocTypeLabel(t *testing.T) {
	cases := map[domain.DocumentType]string{
		domain.FT: "Fatura",
		domain.FS: "Fatura Simplificada",
		domain.FR: "Fatura-Recibo",
		domain.NC: "Nota de Crédito",
		domain.ND: "Nota de Débito",
		domain.GT: "Guia de Transporte",
		domain.GR: "Guia de Remessa",
		domain.GA: "Guia de Movimentação de Ativos Próprios",
		domain.GC: "Guia de Consignação",
		domain.GD: "Guia de Devolução",
		domain.OR: "Orçamento",
		domain.PF: "Fatura Pró-Forma",
		domain.NE: "Nota de Encomenda",
		domain.CM: "Consulta de Mesa",
		domain.FC: "Fatura de Consignação",
		domain.FO: "Folha de Obra",
		domain.OU: "Outro Documento",
		domain.RC: "Recibo",
		domain.RG: "Recibo",
	}
	for dt, want := range cases {
		if got := docTypeLabel(dt); got != want {
			t.Errorf("docTypeLabel(%s) = %q, want %q", dt, got, want)
		}
	}
	if got := docTypeLabel(domain.DocumentType("XX")); got != "XX" {
		t.Errorf("fallback = %q, want %q", got, "XX")
	}
}
