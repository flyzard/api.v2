package saft

import "github.com/flyzard/invoicing.v2/internal/domain"

// xmlLine mirrors the SalesInvoice and WorkingDocument Line sequence
// (identical between the two families per the XSD). MovementOfGoods uses
// its own narrower xmlMovementLine.
//
// DebitAmount / CreditAmount are pointers so encoding/xml emits exactly
// one of them (XSD xs:choice).
type xmlLine struct {
	LineNumber         int            `xml:"LineNumber"`
	OrderReferences    []xmlOrderRef  `xml:"OrderReferences,omitempty"`
	ProductCode        string         `xml:"ProductCode"`
	ProductDescription string         `xml:"ProductDescription"`
	Quantity           saftQty        `xml:"Quantity"`
	UnitOfMeasure      string         `xml:"UnitOfMeasure"`
	UnitPrice          saftMoneyLine  `xml:"UnitPrice"`
	TaxPointDate       string         `xml:"TaxPointDate"`
	References         []xmlDocRef    `xml:"References,omitempty"`
	Description        string         `xml:"Description"`
	DebitAmount        *saftMoneyLine `xml:"DebitAmount,omitempty"`
	CreditAmount       *saftMoneyLine `xml:"CreditAmount,omitempty"`
	Tax                *xmlTax        `xml:"Tax,omitempty"`
	TaxExemptionReason string         `xml:"TaxExemptionReason,omitempty"`
	TaxExemptionCode   string         `xml:"TaxExemptionCode,omitempty"`
	SettlementAmount   *saftMoneyLine `xml:"SettlementAmount,omitempty"`
}

type xmlOrderRef struct {
	OriginatingON string `xml:"OriginatingON,omitempty"`
	OrderDate     string `xml:"OrderDate,omitempty"`
}

type xmlDocRef struct {
	Reference string `xml:"Reference,omitempty"`
	Reason    string `xml:"Reason,omitempty"`
}

type xmlTax struct {
	TaxType          string `xml:"TaxType"`
	TaxCountryRegion string `xml:"TaxCountryRegion"`
	TaxCode          string `xml:"TaxCode"`
	TaxPercentage    string `xml:"TaxPercentage,omitempty"`
	TaxAmount        string `xml:"TaxAmount,omitempty"`
}

// xmlSimpleTotals is the DocumentTotals shape used by MovementOfGoods and
// WorkingDocuments (XSD: TaxPayable, NetTotal, GrossTotal, Currency?). Sales
// has a wider variant (xmlDocumentTotals) with Settlement and Payment.
type xmlSimpleTotals struct {
	TaxPayable saftMoney    `xml:"TaxPayable"`
	NetTotal   saftMoney    `xml:"NetTotal"`
	GrossTotal saftMoney    `xml:"GrossTotal"`
	Currency   *xmlCurrency `xml:"Currency,omitempty"`
}

// lineSide labels which of DebitAmount/CreditAmount this family populates.
type lineSide int

const (
	sideCredit lineSide = iota
	sideDebit
)

func buildLine(l domain.DocumentLine, side lineSide) xmlLine {
	net := saftMoneyLine(l.LineNetAmount())
	out := xmlLine{
		LineNumber:         l.LineNumber,
		OrderReferences:    buildOrderRefs(l.OrderReferences),
		ProductCode:        l.Product.ProductCode,
		ProductDescription: l.Product.ProductDescription,
		Quantity:           saftQty(l.Quantity),
		UnitOfMeasure:      string(l.Product.Unit),
		UnitPrice:          saftMoneyLine(l.EffectiveUnitPrice()),
		TaxPointDate:       fmtDate(l.TaxPointDate),
		References:         buildDocRefs(l.References),
		Description:        l.Product.ProductDescription,
	}
	if side == sideDebit {
		out.DebitAmount = &net
	} else {
		out.CreditAmount = &net
	}
	if disc := l.LineDiscountAmount(); disc > 0 {
		v := saftMoneyLine(disc)
		out.SettlementAmount = &v
	}
	if l.Tax != nil {
		out.Tax, out.TaxExemptionReason, out.TaxExemptionCode = buildTax(l.Tax)
	}
	return out
}

func buildOrderRefs(refs []domain.OrderReference) []xmlOrderRef {
	return mapSlice(refs, func(r domain.OrderReference) xmlOrderRef {
		return xmlOrderRef{
			OriginatingON: r.OriginatingON,
			OrderDate:     optDate(r.OrderDate),
		}
	})
}

func buildDocRefs(refs []domain.DocReference) []xmlDocRef {
	return mapSlice(refs, func(r domain.DocReference) xmlDocRef {
		return xmlDocRef{Reference: r.Reference, Reason: r.Reason}
	})
}

// buildTax dispatches on the LineTax sealed sum and returns the Tax element
// plus the optional sibling fields TaxExemptionReason / TaxExemptionCode
// (which sit outside the Tax element in the XSD).
func buildTax(t domain.LineTax) (*xmlTax, string, string) {
	switch v := t.(type) {
	case domain.VATTax:
		tax := &xmlTax{
			TaxType:          "IVA",
			TaxCountryRegion: string(v.Rate.Region),
			TaxCode:          string(v.Rate.Category),
			TaxPercentage:    fmtPercent(v.Rate.Value),
		}
		if v.Rate.Category == domain.TaxExempt {
			return tax, v.ExemptReason, string(v.Rate.Exemption)
		}
		return tax, "", ""
	case domain.StampTax:
		tax := &xmlTax{
			TaxType:          "IS",
			TaxCountryRegion: string(v.Jurisdiction),
			TaxCode:          v.Code,
			TaxAmount:        domain.Money(v.Amount).Format2DP(),
		}
		return tax, "", ""
	case domain.NotSubjectTax:
		tax := &xmlTax{
			TaxType:          "NS",
			TaxCountryRegion: string(v.Jurisdiction),
			TaxCode:          string(v.Reason),
			TaxPercentage:    "0.00",
		}
		return tax, v.ReasonText, string(v.Reason)
	}
	return nil, "", ""
}
