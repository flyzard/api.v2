package saft

import (
	"fmt"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// xmlSalesInvoices mirrors SAF-T SourceDocuments/SalesInvoices. The family
// aggregates NumberOfEntries / TotalDebit / TotalCredit are computed by
// buildSalesInvoices; TotalDebit folds NC NetTotals, TotalCredit folds
// FT/FS/FR/ND NetTotals (issuer perspective).
type xmlSalesInvoices struct {
	NumberOfEntries int          `xml:"NumberOfEntries"`
	TotalDebit      saftMoney    `xml:"TotalDebit"`
	TotalCredit     saftMoney    `xml:"TotalCredit"`
	Invoices        []xmlInvoice `xml:"Invoice"`
}

type xmlInvoice struct {
	InvoiceNo       string              `xml:"InvoiceNo"`
	ATCUD           string              `xml:"ATCUD"`
	DocumentStatus  xmlDocumentStatus   `xml:"DocumentStatus"`
	Hash            string              `xml:"Hash"`
	HashControl     string              `xml:"HashControl"`
	InvoiceDate     string              `xml:"InvoiceDate"`
	InvoiceType     string              `xml:"InvoiceType"`
	SpecialRegimes  xmlSpecialRegimes   `xml:"SpecialRegimes"`
	SourceID        string              `xml:"SourceID"`
	EACCode         string              `xml:"EACCode,omitempty"`
	SystemEntryDate string              `xml:"SystemEntryDate"`
	CustomerID      string              `xml:"CustomerID"`
	Lines           []xmlLine           `xml:"Line"`
	DocumentTotals  xmlDocumentTotals   `xml:"DocumentTotals"`
	WithholdingTax  []xmlWithholdingTax `xml:"WithholdingTax,omitempty"`
}

type xmlDocumentStatus struct {
	InvoiceStatus     string `xml:"InvoiceStatus"`
	InvoiceStatusDate string `xml:"InvoiceStatusDate"`
	Reason            string `xml:"Reason,omitempty"`
	SourceID          string `xml:"SourceID"`
	SourceBilling     string `xml:"SourceBilling"`
}

type xmlSpecialRegimes struct {
	SelfBilling  int `xml:"SelfBillingIndicator"`
	CashVAT      int `xml:"CashVATSchemeIndicator"`
	ThirdParties int `xml:"ThirdPartiesBillingIndicator"`
}

type xmlDocumentTotals struct {
	TaxPayable saftMoney       `xml:"TaxPayable"`
	NetTotal   saftMoney       `xml:"NetTotal"`
	GrossTotal saftMoney       `xml:"GrossTotal"`
	Currency   *xmlCurrency    `xml:"Currency,omitempty"`
	Settlement []xmlSettlement `xml:"Settlement,omitempty"`
	Payment    []xmlFRPayment  `xml:"Payment,omitempty"`
}

type xmlCurrency struct {
	CurrencyCode   string `xml:"CurrencyCode"`
	CurrencyAmount string `xml:"CurrencyAmount"`
	ExchangeRate   string `xml:"ExchangeRate"`
}

// xmlSettlement holds future payment terms. AT cert §5.7 (round 3348) is
// explicit that this element must not carry the sum of line discounts.
type xmlSettlement struct {
	PaymentTerms string `xml:"PaymentTerms,omitempty"`
}

type xmlFRPayment struct {
	PaymentMechanism string    `xml:"PaymentMechanism"`
	PaymentAmount    saftMoney `xml:"PaymentAmount"`
	PaymentDate      string    `xml:"PaymentDate"`
}

type xmlWithholdingTax struct {
	WithholdingTaxType        string    `xml:"WithholdingTaxType,omitempty"`
	WithholdingTaxDescription string    `xml:"WithholdingTaxDescription,omitempty"`
	WithholdingTaxAmount      saftMoney `xml:"WithholdingTaxAmount"`
}

func buildSalesInvoices(sales []domain.SalesInvoice, issuerEAC string) xmlSalesInvoices {
	invoices := make([]xmlInvoice, 0, len(sales))
	var debit, credit domain.Money
	for _, d := range sales {
		invoices = append(invoices, buildInvoice(d, issuerEAC))
		// TotalDebit/TotalCredit exclude InvoiceStatus "A" and "F" documents
		// (Portaria 302/2016 fields 4.1.2/4.1.3); they stay listed and counted
		// in NumberOfEntries (4.1.1).
		if d.Status == domain.StatusCancelled || d.Status == domain.StatusBilled {
			continue
		}
		if d.DocumentType == domain.NC {
			debit += d.Totals.NetTotal
		} else {
			credit += d.Totals.NetTotal
		}
	}
	sortByKey(invoices, func(i xmlInvoice) string { return i.InvoiceNo })
	return xmlSalesInvoices{
		NumberOfEntries: len(sales),
		TotalDebit:      saftMoney(debit),
		TotalCredit:     saftMoney(credit),
		Invoices:        invoices,
	}
}

func buildInvoice(d domain.SalesInvoice, issuerEAC string) xmlInvoice {
	side := sideCredit
	if d.DocumentType == domain.NC {
		side = sideDebit
	}
	lines := mapSlice(d.Lines, func(l domain.DocumentLine) xmlLine {
		return buildLine(l, side)
	})
	return xmlInvoice{
		InvoiceNo:       d.Number.Format(),
		ATCUD:           string(d.ATCUD),
		DocumentStatus:  buildDocumentStatus(d.IssuedDocument),
		Hash:            string(d.Hash),
		HashControl:     string(d.HashControl),
		InvoiceDate:     fmtDate(d.Date),
		InvoiceType:     string(d.DocumentType),
		SpecialRegimes:  buildSpecialRegimes(d.SpecialRegimes),
		SourceID:        d.SourceID,
		EACCode:         issuerEAC,
		SystemEntryDate: fmtDateTime(d.SystemEntryDate),
		CustomerID:      saftCustomerID(d.Customer.CustomerID),
		Lines:           lines,
		DocumentTotals:  buildSalesTotals(d),
		WithholdingTax:  buildWithholding(d.WithholdingTax),
	}
}

func buildDocumentStatus(d domain.IssuedDocument) xmlDocumentStatus {
	return xmlDocumentStatus{
		InvoiceStatus:     string(d.Status),
		InvoiceStatusDate: fmtDateTime(d.StatusDate),
		Reason:            d.Reason,
		SourceID:          d.SourceID,
		SourceBilling:     string(d.SourceBilling),
	}
}

func buildSpecialRegimes(r domain.SpecialRegimes) xmlSpecialRegimes {
	return xmlSpecialRegimes{
		SelfBilling:  boolToInt(r.SelfBilling),
		CashVAT:      boolToInt(r.CashVAT),
		ThirdParties: boolToInt(r.ThirdParties),
	}
}

func buildSalesTotals(d domain.SalesInvoice) xmlDocumentTotals {
	out := xmlDocumentTotals{
		TaxPayable: saftMoney(d.Totals.TaxTotal + d.Totals.StampDuty),
		NetTotal:   saftMoney(d.Totals.NetTotal),
		GrossTotal: saftMoney(d.Totals.GrossTotal),
	}
	if d.PaymentTerms != nil {
		out.Settlement = []xmlSettlement{{PaymentTerms: fmtDate(*d.PaymentTerms)}}
	}
	if d.Currency != nil {
		out.Currency = buildCurrency(*d.Currency)
	}
	for _, p := range d.Payments {
		out.Payment = append(out.Payment, xmlFRPayment{
			PaymentMechanism: string(p.Mechanism),
			PaymentAmount:    saftMoney(p.Amount),
			PaymentDate:      fmtDate(p.Date),
		})
	}
	return out
}

func buildCurrency(c domain.Currency) *xmlCurrency {
	rate := c.ExchangeRate.Float64()
	return &xmlCurrency{
		CurrencyCode:   string(c.Code),
		CurrencyAmount: fmt.Sprintf("%.2f", c.Amount.Float64()*rate),
		ExchangeRate:   fmt.Sprintf("%.6f", rate),
	}
}

func buildWithholding(ws []domain.WithholdingTax) []xmlWithholdingTax {
	return mapSlice(ws, func(w domain.WithholdingTax) xmlWithholdingTax {
		return xmlWithholdingTax{
			WithholdingTaxType:        string(w.Type),
			WithholdingTaxDescription: w.Description,
			WithholdingTaxAmount:      saftMoney(w.Amount),
		}
	})
}
