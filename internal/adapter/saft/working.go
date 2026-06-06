package saft

import "github.com/flyzard/invoicing.v2/internal/domain"

// xmlWorkingDocuments mirrors SAF-T SourceDocuments/WorkingDocuments.
// Working docs always credit the issuer: TotalDebit is always 0.
type xmlWorkingDocuments struct {
	NumberOfEntries int               `xml:"NumberOfEntries"`
	TotalDebit      saftMoney         `xml:"TotalDebit"`
	TotalCredit     saftMoney         `xml:"TotalCredit"`
	Documents       []xmlWorkDocument `xml:"WorkDocument"`
}

type xmlWorkDocument struct {
	DocumentNumber  string          `xml:"DocumentNumber"`
	ATCUD           string          `xml:"ATCUD"`
	DocumentStatus  xmlWorkStatus   `xml:"DocumentStatus"`
	Hash            string          `xml:"Hash"`
	HashControl     string          `xml:"HashControl"`
	WorkDate        string          `xml:"WorkDate"`
	WorkType        string          `xml:"WorkType"`
	SourceID        string          `xml:"SourceID"`
	EACCode         string          `xml:"EACCode,omitempty"`
	SystemEntryDate string          `xml:"SystemEntryDate"`
	CustomerID      string          `xml:"CustomerID"`
	Lines           []xmlLine       `xml:"Line"`
	DocumentTotals  xmlSimpleTotals `xml:"DocumentTotals"`
}

type xmlWorkStatus struct {
	WorkStatus     string `xml:"WorkStatus"`
	WorkStatusDate string `xml:"WorkStatusDate"`
	Reason         string `xml:"Reason,omitempty"`
	SourceID       string `xml:"SourceID"`
	SourceBilling  string `xml:"SourceBilling"`
}

func buildWorkingDocuments(work []domain.WorkDocument, issuerEAC string) xmlWorkingDocuments {
	docs := make([]xmlWorkDocument, 0, len(work))
	var credit domain.Money
	for _, d := range work {
		docs = append(docs, buildWorkDocument(d, issuerEAC))
		// TotalDebit/TotalCredit exclude only WorkStatus "A" documents
		// (Portaria 302/2016 fields 4.3.2/4.3.3) — unlike SalesInvoices,
		// "F" (faturado) work documents stay in the sums.
		if d.Status == domain.StatusCancelled {
			continue
		}
		credit += d.Totals.NetTotal
	}
	sortByKey(docs, func(d xmlWorkDocument) string { return d.DocumentNumber })
	return xmlWorkingDocuments{
		NumberOfEntries: len(work),
		TotalCredit:     saftMoney(credit),
		Documents:       docs,
	}
}

func buildWorkDocument(d domain.WorkDocument, issuerEAC string) xmlWorkDocument {
	lines := mapSlice(d.Lines, func(l domain.DocumentLine) xmlLine {
		return buildLine(l, sideCredit)
	})
	return xmlWorkDocument{
		DocumentNumber:  d.Number.Format(),
		ATCUD:           string(d.ATCUD),
		DocumentStatus:  buildWorkStatus(d.IssuedDocument),
		Hash:            string(d.Hash),
		HashControl:     string(d.HashControl),
		WorkDate:        fmtDate(d.Date),
		WorkType:        string(d.DocumentType),
		SourceID:        d.SourceID,
		EACCode:         issuerEAC,
		SystemEntryDate: fmtDateTime(d.SystemEntryDate),
		CustomerID:      saftCustomerID(d.Customer.CustomerID),
		Lines:           lines,
		DocumentTotals:  buildSimpleTotals(d.Totals),
	}
}

func buildWorkStatus(d domain.IssuedDocument) xmlWorkStatus {
	return xmlWorkStatus{
		WorkStatus:     string(d.Status),
		WorkStatusDate: fmtDateTime(d.StatusDate),
		Reason:         d.Reason,
		SourceID:       d.SourceID,
		SourceBilling:  string(d.SourceBilling),
	}
}
