package saft

import "github.com/flyzard/invoicing.v2/internal/domain"

// xmlPayments mirrors SAF-T SourceDocuments/Payments. Family aggregates
// walk each PaymentLine.Movement to pick the Debit vs Credit side.
type xmlPayments struct {
	NumberOfEntries int          `xml:"NumberOfEntries"`
	TotalDebit      saftMoney    `xml:"TotalDebit"`
	TotalCredit     saftMoney    `xml:"TotalCredit"`
	Payments        []xmlPayment `xml:"Payment"`
}

type xmlPayment struct {
	PaymentRefNo    string              `xml:"PaymentRefNo"`
	ATCUD           string              `xml:"ATCUD"`
	Period          int                 `xml:"Period"`
	TransactionID   string              `xml:"TransactionID,omitempty"`
	TransactionDate string              `xml:"TransactionDate"`
	PaymentType     string              `xml:"PaymentType"`
	Description     string              `xml:"Description,omitempty"`
	SystemID        string              `xml:"SystemID,omitempty"`
	DocumentStatus  xmlPaymentStatus    `xml:"DocumentStatus"`
	PaymentMethod   []xmlPaymentMethod  `xml:"PaymentMethod"`
	SourceID        string              `xml:"SourceID"`
	SystemEntryDate string              `xml:"SystemEntryDate"`
	CustomerID      string              `xml:"CustomerID"`
	Lines           []xmlPaymentLine    `xml:"Line"`
	DocumentTotals  xmlPaymentTotals    `xml:"DocumentTotals"`
	WithholdingTax  []xmlWithholdingTax `xml:"WithholdingTax,omitempty"`
}

type xmlPaymentStatus struct {
	PaymentStatus     string `xml:"PaymentStatus"`
	PaymentStatusDate string `xml:"PaymentStatusDate"`
	Reason            string `xml:"Reason,omitempty"`
	SourceID          string `xml:"SourceID"`
	SourcePayment     string `xml:"SourcePayment"`
}

type xmlPaymentMethod struct {
	PaymentMechanism string    `xml:"PaymentMechanism,omitempty"`
	PaymentAmount    saftMoney `xml:"PaymentAmount"`
	PaymentDate      string    `xml:"PaymentDate"`
}

type xmlPaymentLine struct {
	LineNumber         int                   `xml:"LineNumber"`
	SourceDocumentID   []xmlSourceDocumentID `xml:"SourceDocumentID"`
	SettlementAmount   *saftMoneyLine        `xml:"SettlementAmount,omitempty"`
	DebitAmount        *saftMoneyLine        `xml:"DebitAmount,omitempty"`
	CreditAmount       *saftMoneyLine        `xml:"CreditAmount,omitempty"`
	Tax                *xmlTax               `xml:"Tax,omitempty"`
	TaxExemptionReason string                `xml:"TaxExemptionReason,omitempty"`
	TaxExemptionCode   string                `xml:"TaxExemptionCode,omitempty"`
}

type xmlSourceDocumentID struct {
	OriginatingON string `xml:"OriginatingON"`
	InvoiceDate   string `xml:"InvoiceDate"`
	Description   string `xml:"Description,omitempty"`
}

// xmlPaymentTotals is narrower than xmlDocumentTotals — payments don't carry
// Settlement or Payment children on their totals block. Currency could be
// added when a scenario needs it.
type xmlPaymentTotals struct {
	TaxPayable saftMoney `xml:"TaxPayable"`
	NetTotal   saftMoney `xml:"NetTotal"`
	GrossTotal saftMoney `xml:"GrossTotal"`
}

func buildPayments(payments []domain.Payment) xmlPayments {
	out := make([]xmlPayment, 0, len(payments))
	var debit, credit domain.Money
	for _, p := range payments {
		out = append(out, buildPayment(p))
		// TotalDebit/TotalCredit exclude cancelled documents (Portaria 302/2016
		// field rules); cancelled payments stay listed and counted in NumberOfEntries.
		if p.Status == domain.StatusCancelled {
			continue
		}
		for _, l := range p.Lines {
			if l.Movement == nil {
				continue
			}
			switch l.Movement.(type) {
			case domain.DebitAmount:
				debit += l.Movement.Amount()
			case domain.CreditAmount:
				credit += l.Movement.Amount()
			}
		}
	}
	sortByKey(out, func(p xmlPayment) string { return p.PaymentRefNo })
	return xmlPayments{
		NumberOfEntries: len(payments),
		TotalDebit:      saftMoney(debit),
		TotalCredit:     saftMoney(credit),
		Payments:        out,
	}
}

func buildPayment(p domain.Payment) xmlPayment {
	methods := mapSlice(p.Methods, func(m domain.PaymentMethod) xmlPaymentMethod {
		return xmlPaymentMethod{
			PaymentMechanism: string(m.Mechanism),
			PaymentAmount:    saftMoney(m.Amount),
			PaymentDate:      fmtDate(m.Date),
		}
	})
	lines := mapSlice(p.Lines, buildPaymentLine)
	return xmlPayment{
		PaymentRefNo:    p.Number.Format(),
		ATCUD:           string(p.ATCUD),
		Period:          int(p.Period),
		TransactionID:   p.TransactionID,
		TransactionDate: fmtDate(p.TransactionDate),
		PaymentType:     string(p.Type),
		Description:     p.Description,
		SystemID:        p.SystemID,
		DocumentStatus: xmlPaymentStatus{
			PaymentStatus:     string(p.Status),
			PaymentStatusDate: fmtDateTime(p.StatusDate),
			Reason:            p.Reason,
			SourceID:          p.SourceID,
			SourcePayment:     string(p.SourcePayment),
		},
		PaymentMethod:   methods,
		SourceID:        p.SourceID,
		SystemEntryDate: fmtDateTime(p.SystemEntryDate),
		CustomerID:      p.Customer.CustomerID.String(),
		Lines:           lines,
		DocumentTotals: xmlPaymentTotals{
			TaxPayable: saftMoney(p.TaxPayable),
			NetTotal:   saftMoney(p.NetTotal),
			GrossTotal: saftMoney(p.GrossTotal),
		},
		WithholdingTax: buildWithholding(p.WithholdingTax),
	}
}

func buildPaymentLine(l domain.PaymentLine) xmlPaymentLine {
	out := xmlPaymentLine{
		LineNumber: l.LineNumber,
		SourceDocumentID: mapSlice(l.SourceDocuments, func(s domain.SourceDocumentID) xmlSourceDocumentID {
			return xmlSourceDocumentID{
				OriginatingON: s.OriginatingON,
				InvoiceDate:   fmtDate(s.InvoiceDate),
				Description:   s.Description,
			}
		}),
	}
	if l.SettlementAmount != nil {
		s := saftMoneyLine(*l.SettlementAmount)
		out.SettlementAmount = &s
	}
	if l.Movement != nil {
		amt := saftMoneyLine(l.Movement.Amount())
		switch l.Movement.(type) {
		case domain.DebitAmount:
			out.DebitAmount = &amt
		case domain.CreditAmount:
			out.CreditAmount = &amt
		}
	}
	if l.Tax != nil {
		out.Tax, out.TaxExemptionReason, out.TaxExemptionCode = buildTax(l.Tax)
	}
	return out
}
