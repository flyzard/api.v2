package at

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Well-known fatcorews endpoints per the AT manual "e-Fatura — Comunicação
// por webservice, Aspetos genéricos" §2.1.2/§2.1.4 (test :723, production
// :423). NOTE: v1 carried :700/:400 — wrong; :700 answers every request
// with a generic "Internal Error" 500 fault.
const (
	TestInvoiceURL       = "https://servicos.portaldasfinancas.gov.pt:723/fatcorews/ws/"
	ProductionInvoiceURL = "https://servicos.portaldasfinancas.gov.pt:423/fatcorews/ws/"
)

// fatcorewsNS is the SOAP body namespace for Fatcorews operations.
// Schema per AT WSDL "Fatcorews" + manual "Comunicação dos elementos dos
// documentos de Faturação à AT - Webservice (Aspetos Específicos)".
const fatcorewsNS = "http://factemi.at.min_financas.pt/documents"

type registerInvoiceRequest struct {
	XMLName                   xml.Name        `xml:"doc:RegisterInvoiceRequest"`
	DocNS                     string          `xml:"xmlns:doc,attr"`
	EFaturaMDVersion          string          `xml:"doc:eFaturaMDVersion"`
	AuditFileVersion          string          `xml:"doc:AuditFileVersion"`
	TaxRegistrationNumber     string          `xml:"doc:TaxRegistrationNumber"`
	TaxEntity                 string          `xml:"doc:TaxEntity"`
	SoftwareCertificateNumber int             `xml:"doc:SoftwareCertificateNumber"`
	InvoiceData               invoiceDataElem `xml:"doc:InvoiceData"`
}

type invoiceDataElem struct {
	InvoiceNo              string             `xml:"doc:InvoiceNo"`
	ATCUD                  string             `xml:"doc:ATCUD"`
	InvoiceDate            string             `xml:"doc:InvoiceDate"`
	InvoiceType            string             `xml:"doc:InvoiceType"`
	SelfBillingIndicator   int                `xml:"doc:SelfBillingIndicator"`
	CustomerTaxID          string             `xml:"doc:CustomerTaxID"`
	CustomerTaxIDCountry   string             `xml:"doc:CustomerTaxIDCountry"`
	DocumentStatus         documentStatusElem `xml:"doc:DocumentStatus"`
	HashCharacters         string             `xml:"doc:HashCharacters"`
	CashVATSchemeIndicator int                `xml:"doc:CashVATSchemeIndicator"`
	PaperLessIndicator     int                `xml:"doc:PaperLessIndicator"`
	EACCode                string             `xml:"doc:EACCode,omitempty"`
	SystemEntryDate        string             `xml:"doc:SystemEntryDate"`
	LineSummaries          []lineSummaryElem  `xml:"doc:LineSummary"`
	DocumentTotals         documentTotalsElem `xml:"doc:DocumentTotals"`
}

type documentStatusElem struct {
	InvoiceStatus     string `xml:"doc:InvoiceStatus"`
	InvoiceStatusDate string `xml:"doc:InvoiceStatusDate"`
}

type lineSummaryElem struct {
	TaxPointDate         string  `xml:"doc:TaxPointDate"`
	DebitCreditIndicator string  `xml:"doc:DebitCreditIndicator"`
	Amount               string  `xml:"doc:Amount"`
	Tax                  taxElem `xml:"doc:Tax"`
	TaxExemptionCode     string  `xml:"doc:TaxExemptionCode,omitempty"`
}

type taxElem struct {
	TaxType          string `xml:"doc:TaxType"`
	TaxCountryRegion string `xml:"doc:TaxCountryRegion"`
	TaxCode          string `xml:"doc:TaxCode"`
	TaxPercentage    string `xml:"doc:TaxPercentage,omitempty"`
	TotalTaxAmount   string `xml:"doc:TotalTaxAmount,omitempty"`
}

type documentTotalsElem struct {
	TaxPayable string `xml:"doc:TaxPayable"`
	NetTotal   string `xml:"doc:NetTotal"`
	GrossTotal string `xml:"doc:GrossTotal"`
}

type registerInvoiceResponse struct {
	XMLName  xml.Name             `xml:"RegisterInvoiceResponse"`
	Response invoiceResponseInner `xml:"Response"`
}

type invoiceResponseInner struct {
	CodigoResposta int    `xml:"CodigoResposta"`
	Mensagem       string `xml:"Mensagem"`
	DataOperacao   string `xml:"DataOperacao"`
}

// InvoiceResult is AT's answer to RegisterInvoice.
type InvoiceResult struct {
	// Code is AT's CodigoResposta; 0 on success (non-zero codes surface as Error instead).
	Code          int
	Message       string
	OperationDate time.Time
}

// invoiceLineGroupKey identifies a LineSummary aggregation bucket: one
// summary per (TaxType, Region, TaxCode) × ExemptionCode × TaxPointDate ×
// DebitCreditIndicator.
type invoiceLineGroupKey struct {
	taxType       string
	region        string
	code          string
	percentage    string
	exemptionCode string
	taxPointDate  string
	debitCredit   string
}

type invoiceLineGroup struct {
	amount    domain.Money
	taxAmount domain.Money
}

func buildInvoiceEnvelope(creds soapCredentials, company domain.Company, inv domain.SalesInvoice) ([]byte, error) {
	if !inv.DocumentType.IsSales() {
		return nil, fmt.Errorf("document %s is not a sales document", inv.Number.Format())
	}
	if inv.Customer.CustomerTaxID == "" {
		return nil, fmt.Errorf("document %s has no customer tax id", inv.Number.Format())
	}

	// Credit notes debit the customer account; all other sales doc types
	// credit it (AT spec DebitCreditIndicator).
	debit := "C"
	if inv.DocumentType == domain.NC {
		debit = "D"
	}

	groups := map[invoiceLineGroupKey]*invoiceLineGroup{}
	var order []invoiceLineGroupKey
	for _, line := range inv.Lines {
		tpd := line.TaxPointDate
		if tpd.IsZero() {
			tpd = inv.Date
		}
		var k invoiceLineGroupKey
		var taxAmt domain.Money
		switch tax := line.Tax.(type) {
		case domain.VATTax:
			k = invoiceLineGroupKey{
				taxType:       "IVA",
				region:        string(tax.Rate.Region),
				code:          string(tax.Rate.Category),
				percentage:    tax.Rate.Value.Format2DP(),
				exemptionCode: string(tax.Rate.Exemption),
				taxPointDate:  tpd.Format("2006-01-02"),
				debitCredit:   debit,
			}
		case domain.StampTax:
			k = invoiceLineGroupKey{
				taxType:      "IS",
				region:       string(tax.Jurisdiction),
				code:         tax.Code,
				taxPointDate: tpd.Format("2006-01-02"),
				debitCredit:  debit,
			}
			taxAmt = tax.Amount
		case domain.NotSubjectTax:
			k = invoiceLineGroupKey{
				taxType:       "NS",
				region:        string(tax.Jurisdiction),
				code:          string(tax.Reason),
				percentage:    "0.00",
				exemptionCode: string(tax.Reason),
				taxPointDate:  tpd.Format("2006-01-02"),
				debitCredit:   debit,
			}
		default:
			// LineTax is a sealed sum (VATTax | StampTax | NotSubjectTax); this branch
			// is only reached when Tax is nil.
			return nil, fmt.Errorf("line %d: unsupported tax shape %T for invoice communication", line.LineNumber, line.Tax)
		}
		g, ok := groups[k]
		if !ok {
			g = &invoiceLineGroup{}
			groups[k] = g
			order = append(order, k)
		}
		g.amount = g.amount.Add(line.LineNetAmount())
		g.taxAmount = g.taxAmount.Add(taxAmt)
	}

	summaries := make([]lineSummaryElem, 0, len(order))
	for _, k := range order {
		g := groups[k]
		elem := lineSummaryElem{
			TaxPointDate:         k.taxPointDate,
			DebitCreditIndicator: k.debitCredit,
			Amount:               g.amount.Format2DP(),
			Tax: taxElem{
				TaxType:          k.taxType,
				TaxCountryRegion: k.region,
				TaxCode:          k.code,
			},
		}
		switch k.taxType {
		case "IVA":
			elem.Tax.TaxPercentage = k.percentage
			if k.code == string(domain.TaxExempt) {
				elem.TaxExemptionCode = k.exemptionCode
			}
		case "NS":
			elem.Tax.TaxPercentage = k.percentage
			elem.TaxExemptionCode = k.exemptionCode
		default: // "IS"
			elem.Tax.TotalTaxAmount = g.taxAmount.Format2DP()
		}
		summaries = append(summaries, elem)
	}

	// HashCharacters: "0" when uncertified (SoftwareCertNum=0), else the
	// 1st/11th/21st/31st chars of the document hash. AT cross-validates
	// against SoftwareCertificateNumber and rejects inconsistent values.
	hashChars := "0"
	if creds.SoftwareCertNum != 0 {
		hashChars = inv.Hash.FourChars()
	}

	selfBill := 0
	if inv.Status == domain.StatusSelfBilled {
		selfBill = 1
	}

	customerCountry := string(inv.Customer.BillingAddress.Country)
	if customerCountry == "" {
		customerCountry = "PT"
	}

	body := registerInvoiceRequest{
		DocNS:                     fatcorewsNS,
		EFaturaMDVersion:          "0.0.1",
		AuditFileVersion:          "1.04_01",
		TaxRegistrationNumber:     string(company.NIF),
		TaxEntity:                 creds.TaxEntity,
		SoftwareCertificateNumber: creds.SoftwareCertNum,
		InvoiceData: invoiceDataElem{
			InvoiceNo:            inv.Number.Format(),
			ATCUD:                string(inv.ATCUD),
			InvoiceDate:          inv.Date.Format("2006-01-02"),
			InvoiceType:          string(inv.DocumentType),
			SelfBillingIndicator: selfBill,
			CustomerTaxID:        string(inv.Customer.CustomerTaxID),
			CustomerTaxIDCountry: customerCountry,
			DocumentStatus: documentStatusElem{
				InvoiceStatus:     string(inv.Status),
				InvoiceStatusDate: inv.StatusDate.Format("2006-01-02T15:04:05"),
			},
			HashCharacters:         hashChars,
			CashVATSchemeIndicator: 0,
			PaperLessIndicator:     0, // PaperLessIndicator 0 per the AT manual's example (v3.0 Oct 2025 §2.1.1.3); 1 requires actual paperless archival.
			EACCode:                company.EACCode,
			SystemEntryDate:        inv.SystemEntryDate.Format("2006-01-02T15:04:05"),
			LineSummaries:          summaries,
			DocumentTotals: documentTotalsElem{
				TaxPayable: inv.Totals.TaxTotal.Format2DP(),
				NetTotal:   inv.Totals.NetTotal.Format2DP(),
				GrossTotal: inv.Totals.GrossTotal.Format2DP(),
			},
		},
	}
	return buildSOAPEnvelope(creds, body)
}

// CommunicateInvoice submits a sales document's data to AT in real time
// (fatcorews RegisterInvoice, DL 28/2019 channel).
func (c *Client) CommunicateInvoice(ctx context.Context, company domain.Company, inv domain.SalesInvoice) (*InvoiceResult, error) {
	if c.config.InvoiceURL == "" {
		return nil, fmt.Errorf("at.Config: InvoiceURL required for CommunicateInvoice")
	}
	ctx, cancel := c.ensureDeadline(ctx)
	defer cancel()
	return retryable(ctx, c.logger, c.config.Retry, "CommunicateInvoice", func() (*InvoiceResult, error) {
		return c.communicateInvoiceOnce(ctx, company, inv)
	})
}

func (c *Client) communicateInvoiceOnce(ctx context.Context, company domain.Company, inv domain.SalesInvoice) (*InvoiceResult, error) {
	creds, err := c.prepareCredentials()
	if err != nil {
		return nil, err
	}
	envelope, err := buildInvoiceEnvelope(creds, company, inv)
	if err != nil {
		return nil, fmt.Errorf("building SOAP envelope: %w", err)
	}
	respBody, err := c.sendSOAPRequest(ctx, "CommunicateInvoice", c.config.InvoiceURL, envelope)
	if err != nil {
		return nil, err
	}

	var resp registerInvoiceResponse
	if err := parseSOAPResponse(respBody, &resp); err != nil {
		return nil, err
	}
	if resp.Response.CodigoResposta != 0 {
		atErr := Error{Code: strconv.Itoa(resp.Response.CodigoResposta), Message: resp.Response.Mensagem}
		c.logger.WarnContext(ctx, "AT returned error",
			slog.String("operation", "CommunicateInvoice"),
			slog.String("code", atErr.Code),
			slog.String("message", atErr.Message))
		return nil, atErr
	}

	opDate, err := time.Parse("2006-01-02T15:04:05", resp.Response.DataOperacao)
	if err != nil {
		c.logger.WarnContext(ctx, "AT DataOperacao unparseable; using local time",
			slog.String("DataOperacao", resp.Response.DataOperacao))
		opDate = time.Now()
	}
	return &InvoiceResult{
		Code:          0,
		Message:       resp.Response.Mensagem,
		OperationDate: opDate,
	}, nil
}
