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

import (
	"bytes"
	"errors"
	"image/png"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

var (
	ErrMissingSellerName    = errors.New("pdf: seller name is required")
	ErrMissingSellerTaxID   = errors.New("pdf: seller tax id is required")
	ErrMissingSellerAddress = errors.New("pdf: seller address, postal code and city are required")
	ErrMissingCertNumber    = errors.New("pdf: certificate number is required")
	ErrMissingQRPayload     = errors.New("pdf: issued document has empty QR payload")
	ErrInvalidLogoPNG       = errors.New("pdf: LogoPNG is not a decodable PNG")
)

// CopyKind is the legally required copy designation printed on each output.
type CopyKind int

const (
	Original CopyKind = iota
	Duplicado
	Triplicado // transport documents must print three copies (DL 28/2019 Art. 6)
	SegundaVia // reprint
)

func (c CopyKind) label() string {
	switch c {
	case Duplicado:
		return "Duplicado"
	case Triplicado:
		return "Triplicado"
	case SegundaVia:
		return "2.ª via"
	default:
		return "Original"
	}
}

// RequiredVias is the set of copies a first emission must print: every
// document gets "Original" + "Duplicado" (Art. 36 n.º 4 CIVA / DL 28/2019
// Art. 6 n.º 1); transport documents must print three vias. Reprints use
// SegundaVia instead and are the caller's decision.
func RequiredVias(dt domain.DocumentType) []CopyKind {
	if dt.IsTransport() {
		return []CopyKind{Original, Duplicado, Triplicado}
	}
	return []CopyKind{Original, Duplicado}
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
	// The issuer's full address is mandatory print content (Portaria 363/2010
	// Art. 6.º); rejecting it here also keeps fmtAddress free of empty fields.
	if m.Seller.Address == "" || m.Seller.PostalCode == "" || m.Seller.City == "" {
		return ErrMissingSellerAddress
	}
	if m.CertNumber == "" {
		return ErrMissingCertNumber
	}
	// maroto silently renders a blank slot for undecodable image bytes, so a
	// bad logo must be rejected here instead of failing invisibly at print.
	// Full decode (not DecodeConfig): a truncated pixel stream must fail too.
	if m.LogoPNG != nil {
		if _, err := png.Decode(bytes.NewReader(m.LogoPNG)); err != nil {
			return ErrInvalidLogoPNG
		}
	}
	return nil
}
