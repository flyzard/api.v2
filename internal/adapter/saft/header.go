package saft

import (
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// xmlHeader mirrors SAF-T 1.04_01 AuditFile/Header. Element order must match
// the XSD xs:sequence — do not reorder fields without updating the XSD.
type xmlHeader struct {
	AuditFileVersion          string     `xml:"AuditFileVersion"`
	CompanyID                 string     `xml:"CompanyID"`
	TaxRegistrationNumber     string     `xml:"TaxRegistrationNumber"`
	TaxAccountingBasis        string     `xml:"TaxAccountingBasis"`
	CompanyName               string     `xml:"CompanyName"`
	BusinessName              string     `xml:"BusinessName,omitempty"`
	CompanyAddress            xmlAddress `xml:"CompanyAddress"`
	FiscalYear                int        `xml:"FiscalYear"`
	StartDate                 string     `xml:"StartDate"`
	EndDate                   string     `xml:"EndDate"`
	CurrencyCode              string     `xml:"CurrencyCode"`
	DateCreated               string     `xml:"DateCreated"`
	TaxEntity                 string     `xml:"TaxEntity"`
	ProductCompanyTaxID       string     `xml:"ProductCompanyTaxID"`
	SoftwareCertificateNumber string     `xml:"SoftwareCertificateNumber"`
	ProductID                 string     `xml:"ProductID"`
	ProductVersion            string     `xml:"ProductVersion"`
	HeaderComment             string     `xml:"HeaderComment,omitempty"`
	Telephone                 string     `xml:"Telephone,omitempty"`
	Fax                       string     `xml:"Fax,omitempty"`
	Email                     string     `xml:"Email,omitempty"`
	Website                   string     `xml:"Website,omitempty"`
}

// xmlAddress mirrors SAF-T AddressStructure (and AddressStructurePT, which
// adds nothing relevant here). Used for CompanyAddress, BillingAddress, etc.
type xmlAddress struct {
	BuildingNumber string `xml:"BuildingNumber,omitempty"`
	StreetName     string `xml:"StreetName,omitempty"`
	AddressDetail  string `xml:"AddressDetail"`
	City           string `xml:"City"`
	PostalCode     string `xml:"PostalCode"`
	Region         string `xml:"Region,omitempty"`
	Country        string `xml:"Country"`
}

func buildHeader(h Header, basis string) xmlHeader {
	nif := string(h.Issuer.NIF)
	return xmlHeader{
		AuditFileVersion:          auditFileVersion,
		CompanyID:                 nif,
		TaxRegistrationNumber:     nif,
		TaxAccountingBasis:        basis,
		CompanyName:               h.Issuer.Name,
		BusinessName:              h.Issuer.TradeName,
		CompanyAddress:            buildAddress(h.Issuer.Address),
		FiscalYear:                h.Issuer.FiscalYear,
		StartDate:                 fmtDate(h.Start),
		EndDate:                   fmtDate(h.End),
		CurrencyCode:              currencyCodeEUR,
		DateCreated:               fmtDate(h.CreatedAt),
		TaxEntity:                 taxEntity,
		ProductCompanyTaxID:       h.Software.ProducerTaxID,
		SoftwareCertificateNumber: h.Software.CertificateNumber,
		ProductID:                 h.Software.ProductID,
		ProductVersion:            h.Software.Version,
		HeaderComment:             h.HeaderComment,
		Telephone:                 h.Issuer.Phone,
		Fax:                       h.Issuer.Fax,
		Email:                     h.Issuer.Email,
		Website:                   h.Issuer.Website,
	}
}

func buildAddress(a domain.Address) xmlAddress {
	return xmlAddress{
		BuildingNumber: a.BuildingNumber,
		StreetName:     a.StreetName,
		AddressDetail:  a.AddressDetail,
		City:           a.City,
		PostalCode:     a.PostalCode,
		Region:         a.Region,
		Country:        string(a.Country),
	}
}

// SAF-T date formats: dates use ISO calendar; system-entry / status-date
// stamps use ISO calendar + clock per Portaria 363/2010.
func fmtDate(t time.Time) string     { return t.Format("2006-01-02") }
func fmtDateTime(t time.Time) string { return t.Format("2006-01-02T15:04:05") }

// optDate formats an optional *time.Time as a date string; nil → "".
// Combined with `xml:"...,omitempty"` on the target field this drops the
// element entirely when unset.
func optDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmtDate(*t)
}
