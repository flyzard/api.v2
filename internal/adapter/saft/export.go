// Package saft is the SAF-T (PT) XML projector. It consumes typed family values produced by package domain and emits a Windows-1252 XML file that
// validates against SAFTPT_1_04_01.xsd.

package saft

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"golang.org/x/text/encoding/charmap"
)

// SAF-T PT 1.04_01 default namespace.
const saftNamespace = "urn:OECD:StandardAuditFile-Tax:PT_1.04_01"

// SoftwareIdentity is the AT-certified producer/software metadata stamped
// into AuditFile/Header (ProductCompanyTaxID, SoftwareCertificateNumber,
// ProductID, ProductVersion). The caller maps its own config onto this
// struct so the projector stays decoupled from configuration loading.
type SoftwareIdentity struct {
	ProducerTaxID     string
	CertificateNumber string
	ProductID         string // "SoftwareName/ProducerName"
	Version           string
}

// Header holds the source values for AuditFile/Header; buildHeader projects
// to wire format at marshal time so projected fields can't drift from source.
//
// TaxAccountingBasis defaults to "F" (Faturação — the tenant's own billing).
// For self-billing, Portaria 302/2016 alínea g) prescribes a SEPARATE file per
// supplier with basis "S": Header carries the supplier's data (Issuer = the
// supplier), MasterFiles/Customer carries the issuing acquirer, and every
// sales invoice has InvoiceStatus "S". Supplier rows inside an "F" file are
// not how self-billing is represented.
type Header struct {
	Issuer             domain.Company
	Software           SoftwareIdentity
	Start, End         time.Time
	CreatedAt          time.Time
	HeaderComment      string
	TaxAccountingBasis string // "" or "F" = Faturação; "S" = Autofaturação
}

// basis resolves the wire TaxAccountingBasis, restricted to the two file
// kinds this library produces.
func (h Header) basis() (string, error) {
	switch h.TaxAccountingBasis {
	case "", "F":
		return "F", nil
	case "S":
		return "S", nil
	}
	return "", fmt.Errorf("unsupported TaxAccountingBasis %q (only \"F\" and \"S\")", h.TaxAccountingBasis)
}

// SAF-T PT constants per Portaria 195/2020 + 2025 updates.
const (
	auditFileVersion = "1.04_01"
	taxEntity        = "Global"
	currencyCodeEUR  = "EUR"
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
	basis, err := hdr.basis()
	if err != nil {
		return nil, err
	}
	mf, err := buildMasterFiles(sales, stock, work, payments)
	if err != nil {
		return nil, fmt.Errorf("masterfiles: %w", err)
	}
	af := xmlAuditFile{
		Xmlns:       saftNamespace,
		Header:      buildHeader(hdr, basis),
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

// SAF-T PT requires Windows-1252 (Portaria 363/2010, regras §R-G7). We emit
// our own XML declaration so the encoding attribute matches the actual byte
// representation — never use encoding/xml's xml.Header here (it hardcodes
// "UTF-8"). XSD validation out-of-band requires XSD 1.1 (Xerces-J / Saxon EE);
// xmllint can't compile the schema (uses xs:assert + unbounded maxOccurs).
const xmlDeclarationWin1252 = `<?xml version="1.0" encoding="Windows-1252"?>` + "\n"

// transcodeWin1252 converts a UTF-8 buffer to Windows-1252. This is the sole
// enforcement point for the AT charset invariant (Portaria 363/2010 §R-G7) —
// an error here means a text field carried a rune unmappable in Windows-1252,
// and the message names the first offender so the operator can find the field.
func transcodeWin1252(utf8 []byte) ([]byte, error) {
	out, err := charmap.Windows1252.NewEncoder().Bytes(utf8)
	if err != nil {
		return nil, fmt.Errorf("transcode UTF-8 → Windows-1252: %w; %s", err, win1252Offender(utf8))
	}
	return out, nil
}

// win1252Offender locates the first rune Windows-1252 cannot represent and
// describes it with surrounding context. U+FFFD means encoding/xml already
// replaced an XML-invalid control character upstream.
func win1252Offender(b []byte) string {
	for i, r := range string(b) {
		if _, ok := charmap.Windows1252.EncodeRune(r); !ok {
			start := max(0, i-60)
			end := min(len(b), i+60)
			note := ""
			if r == '�' {
				note = " (U+FFFD: an XML-invalid character was replaced upstream by the XML encoder)"
			}
			return fmt.Sprintf("first unmappable rune %q at byte %d%s, context: %q", r, i, note, b[start:end])
		}
	}
	return "offending rune not located"
}
