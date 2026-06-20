package saft

import "github.com/flyzard/invoicing.v2/internal/domain"

// xmlLine mirrors the SalesInvoice and WorkingDocument Line sequence
// (identical between the two families per the XSD). MovementOfGoods uses
// its own narrower xmlMovementLine.
//
// DebitAmount / CreditAmount are pointers so encoding/xml emits exactly
// one of them (XSD xs:choice).
//
// UnitPrice, CreditAmount, and DebitAmount are plain strings so buildLine
// can supply the linePricePair matched values (high-precision UnitPrice +
// exact-product CreditAmount/DebitAmount) without going through saftMoneyLine.
// SettlementAmount remains saftMoneyLine (5dp) — it is the line discount,
// not part of the Quantity × UnitPrice identity.
type xmlLine struct {
	LineNumber         int            `xml:"LineNumber"`
	OrderReferences    []xmlOrderRef  `xml:"OrderReferences,omitempty"`
	ProductCode        string         `xml:"ProductCode"`
	ProductDescription string         `xml:"ProductDescription"`
	Quantity           saftQty        `xml:"Quantity"`
	UnitOfMeasure      string         `xml:"UnitOfMeasure"`
	UnitPrice          string         `xml:"UnitPrice"`
	TaxBase            *saftMoney     `xml:"TaxBase,omitempty"`
	TaxPointDate       string         `xml:"TaxPointDate"`
	References         []xmlDocRef    `xml:"References,omitempty"`
	Description        string         `xml:"Description"`
	DebitAmount        *string        `xml:"DebitAmount,omitempty"`
	CreditAmount       *string        `xml:"CreditAmount,omitempty"`
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

// buildSimpleTotals projects domain Totals into the narrow DocumentTotals
// block shared by working and movement documents. TaxPayable is derived as
// Gross−Net at 2dp so round(Net)+round(Tax)==round(Gross) under sub-cent
// accumulation — the same invariant as buildSalesTotals.
func buildSimpleTotals(t domain.Totals) xmlSimpleTotals {
	return xmlSimpleTotals{
		TaxPayable: saftMoney(domain.MoneyFromCents(t.GrossTotal.Cents() - t.NetTotal.Cents())),
		NetTotal:   saftMoney(t.NetTotal),
		GrossTotal: saftMoney(t.GrossTotal),
	}
}

// SAF-T TaxType discriminators, shared by line Tax elements and the
// MasterFiles TaxTable.
const (
	taxTypeVAT   = "IVA"
	taxTypeStamp = "IS"
	taxTypeNS    = "NS"
)

// lineSide labels which of DebitAmount/CreditAmount this family populates.
type lineSide int

const (
	sideCredit lineSide = iota
	sideDebit
)

func buildLine(l domain.DocumentLine, side lineSide) xmlLine {
	upStr, amtStr := linePricePair(l.LineNetAmount(), l.Quantity)
	out := xmlLine{
		LineNumber:         l.LineNumber,
		OrderReferences:    buildOrderRefs(l.OrderReferences),
		ProductCode:        l.Product.ProductCode,
		ProductDescription: l.Product.ProductDescription,
		Quantity:           saftQty(l.Quantity),
		UnitOfMeasure:      string(l.Product.Unit),
		UnitPrice:          upStr,
		TaxPointDate:       fmtDate(l.TaxPointDate),
		References:         buildDocRefs(l.References),
		Description:        l.Product.ProductDescription,
	}
	if l.TaxBase != nil {
		v := saftMoney(*l.TaxBase)
		out.TaxBase = &v
	}
	if side == sideDebit {
		out.DebitAmount = &amtStr
	} else {
		out.CreditAmount = &amtStr
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
			TaxType:          taxTypeVAT,
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
			TaxType:          taxTypeStamp,
			TaxCountryRegion: string(v.Jurisdiction),
			TaxCode:          v.Code,
			TaxAmount:        domain.Money(v.Amount).Format2DP(),
		}
		return tax, "", ""
	case domain.NotSubjectTax:
		// TaxCode is the SAF-T tax-code enum (RED|INT|NOR|ISE|OUT|NS), not the
		// exemption motive — the Mxx reason rides in TaxExemptionCode below.
		tax := &xmlTax{
			TaxType:          taxTypeNS,
			TaxCountryRegion: string(v.Jurisdiction),
			TaxCode:          taxTypeNS,
			TaxPercentage:    "0.00",
		}
		return tax, v.ReasonText, string(v.Reason)
	}
	return nil, "", ""
}
