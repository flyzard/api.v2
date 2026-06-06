package pdf

import (
	"fmt"
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

// commaDecimal converts a dot-decimal numeric string to the Portuguese comma form.
func commaDecimal(s string) string { return strings.Replace(s, ".", ",", 1) }

// fmtEUR renders Money for print with Portuguese decimal comma: "1234,56 €".
func fmtEUR(m domain.Money) string { return commaDecimal(m.Format2DP()) + " €" }

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

func fmtPercent(p domain.Percent) string {
	return commaDecimal(p.Format2DP()) + "%"
}

// mechanismLabels are the printed names of the SAF-T PaymentMechanism codes.
var mechanismLabels = map[domain.PaymentMechanism]string{
	domain.PaymentMechanismCreditCard:     "Cartão de crédito",
	domain.PaymentMechanismDebitCard:      "Cartão de débito",
	domain.PaymentMechanismCheck:          "Cheque",
	domain.PaymentMechanismIntlCredit:     "Crédito documentário",
	domain.PaymentMechanismGiftCard:       "Cartão/cheque oferta",
	domain.PaymentMechanismBalanceComp:    "Compensação de saldos",
	domain.PaymentMechanismElectronic:     "Dinheiro eletrónico",
	domain.PaymentMechanismCommercialBill: "Letra comercial",
	domain.PaymentMechanismMultibanco:     "Referência Multibanco",
	domain.PaymentMechanismCash:           "Numerário",
	domain.PaymentMechanismOther:          "Outro",
	domain.PaymentMechanismBarter:         "Permuta",
	domain.PaymentMechanismBankTransfer:   "Transferência bancária",
	domain.PaymentMechanismTitleVouchers:  "Títulos de refeição",
}

func mechanismLabel(m domain.PaymentMechanism) string {
	if l, ok := mechanismLabels[m]; ok {
		return l
	}
	return string(m)
}

// currencyLabel prints the native foreign amount via Currency.NativeAmount —
// the same integer-cents rounding the SAF-T CurrencyAmount projection uses
// (saft/sales.go), so the PDF and the XML can never disagree on the figure.
func currencyLabel(c domain.Currency) string {
	return fmt.Sprintf("Moeda original: %s %s · câmbio %s",
		string(c.Code), commaDecimal(c.NativeAmount().Format2DP()),
		commaDecimal(fmt.Sprintf("%.6f", c.ExchangeRate.Float64())))
}
