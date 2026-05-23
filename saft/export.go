// Package saft is the SAF-T (PT) XML projector. It consumes typed family values produced by package domain and emits a Windows-1252 XML file that
// validates against SAFTPT_1_04_01.xsd.
//
// The projector is pure projection: it does not mutate inputs and has no dependencies on cmd. See SAFT_EXPORT_PLAN.md for scope, gaps closed in
// Phase A, and the element mapping in §7.
package saft

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/flyzard/invoicing.v2/domain"
)

// SAF-T PT 1.04_01 default namespace.
const saftNamespace = "urn:OECD:StandardAuditFile-Tax:PT_1.04_01"

// Header holds the source values for AuditFile/Header; buildHeader projects
// to wire format at marshal time so projected fields can't drift from source.
type Header struct {
	Issuer        domain.Company
	Software      domain.SoftwareIdentity
	Start, End    time.Time
	CreatedAt     time.Time
	HeaderComment string
}

// SAF-T PT constants per Portaria 195/2020 + 2025 updates.
const (
	auditFileVersion   = "1.04_01"
	taxAccountingBasis = "F" // Faturação (billing-only)
	taxEntity          = "Global"
	currencyCodeEUR    = "EUR"
)

// xmlAuditFile is the SAF-T root element. SourceDocuments is optional and
// only emitted when at least one family has documents.
type xmlAuditFile struct {
	XMLName         xml.Name            `xml:"AuditFile"`
	Xmlns           string              `xml:"xmlns,attr"`
	Header          xmlHeader           `xml:"Header"`
	MasterFiles     xmlMasterFiles      `xml:"MasterFiles"`
	SourceDocuments *xmlSourceDocuments `xml:"SourceDocuments,omitempty"`
}

type xmlSourceDocuments struct {
	SalesInvoices    *xmlSalesInvoices    `xml:"SalesInvoices,omitempty"`
	MovementOfGoods  *xmlMovementOfGoods  `xml:"MovementOfGoods,omitempty"`
	WorkingDocuments *xmlWorkingDocuments `xml:"WorkingDocuments,omitempty"`
	Payments         *xmlPayments         `xml:"Payments,omitempty"`
}

// Export produces a Windows-1252 XML byte slice ready to write to disk.
func Export(hdr Header,
	sales []domain.SalesInvoice,
	stock []domain.StockMovement,
	work []domain.WorkDocument,
	payments []domain.Payment,
) ([]byte, error) {
	mf, err := buildMasterFiles(sales, stock, work, payments)
	if err != nil {
		return nil, fmt.Errorf("masterfiles: %w", err)
	}
	af := xmlAuditFile{
		Xmlns:       saftNamespace,
		Header:      buildHeader(hdr),
		MasterFiles: mf,
	}
	if len(sales)+len(stock)+len(work)+len(payments) > 0 {
		sd := xmlSourceDocuments{}
		issuerEAC := hdr.Issuer.EACCode
		if len(sales) > 0 {
			v := buildSalesInvoices(sales, issuerEAC)
			sd.SalesInvoices = &v
		}
		if len(stock) > 0 {
			v := buildMovementOfGoods(stock, issuerEAC)
			sd.MovementOfGoods = &v
		}
		if len(work) > 0 {
			v := buildWorkingDocuments(work, issuerEAC)
			sd.WorkingDocuments = &v
		}
		if len(payments) > 0 {
			v := buildPayments(payments)
			sd.Payments = &v
		}
		af.SourceDocuments = &sd
	}

	var buf bytes.Buffer
	buf.WriteString(xmlDeclarationWin1252)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(af); err != nil {
		return nil, fmt.Errorf("marshal AuditFile: %w", err)
	}
	return transcodeWin1252(buf.Bytes())
}
