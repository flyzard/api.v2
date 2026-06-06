package pdf

import (
	"strings"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// docTypeLabels are the printed Portuguese document names.
var docTypeLabels = map[domain.DocumentType]string{
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

func docTypeLabel(dt domain.DocumentType) string {
	if l, ok := docTypeLabels[dt]; ok {
		return l
	}
	return string(dt)
}

// fmtEUR renders Money for print with Portuguese decimal comma: "1234,56 €".
func fmtEUR(m domain.Money) string {
	return strings.Replace(m.Format2DP(), ".", ",", 1) + " €"
}

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

func fmtPercent(p domain.Percent) string {
	return strings.Replace(p.Format2DP(), ".", ",", 1) + "%"
}
