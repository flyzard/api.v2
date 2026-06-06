// Package pdf renders issued domain documents as A4 PDF files satisfying the
// Portuguese AT print requirements: QR code (ECC M, ≥30×30 mm, Portaria
// 195/2020), ATCUD immediately above the QR (Despacho 412/2020-XXII), the
// 4 signature characters + "Processado por programa certificado" mention
// (Portaria 363/2010 Art. 6, Despacho 8632/2014), and per-family legal
// mentions. Fixed single layout, Portuguese labels.
//
// Like package saft, this is a pure projector: it consumes immutable domain
// values plus caller-mapped Meta and never mutates anything.
package pdf

import "errors"

var (
	ErrMissingSellerName  = errors.New("pdf: seller name is required")
	ErrMissingSellerTaxID = errors.New("pdf: seller tax id is required")
	ErrMissingCertNumber  = errors.New("pdf: certificate number is required")
	ErrMissingQRPayload   = errors.New("pdf: issued document has empty QR payload")
)

// CopyKind is the legally required copy designation printed on each output.
type CopyKind int

const (
	Original CopyKind = iota
	Duplicado
	SegundaVia // reprint
)

func (c CopyKind) label() string {
	switch c {
	case Duplicado:
		return "Duplicado"
	case SegundaVia:
		return "2.ª via"
	default:
		return "Original"
	}
}

// Seller is the issuing entity block printed in the header. Caller-mapped,
// like saft.SoftwareIdentity, so the adapter stays decoupled from config.
type Seller struct {
	Name       string
	TaxID      string
	Address    string
	City       string
	PostalCode string
	Phone      string // optional
	Email      string // optional
}

// Meta carries everything the layout needs beyond the domain value itself.
type Meta struct {
	Seller     Seller
	CertNumber string // "Processado por programa certificado n.º <cert>/AT"
	Copy       CopyKind
	LogoPNG    []byte // optional header logo
}

func (m Meta) validate() error {
	if m.Seller.Name == "" {
		return ErrMissingSellerName
	}
	if m.Seller.TaxID == "" {
		return ErrMissingSellerTaxID
	}
	if m.CertNumber == "" {
		return ErrMissingCertNumber
	}
	return nil
}
