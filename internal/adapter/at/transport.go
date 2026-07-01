package at

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Well-known sgdtws endpoints (v1 at_client.go §ATTestEndpoints). No trailing
// slash — the gateway 404s with one on this service.
const (
	TestTransportURL       = "https://servicos.portaldasfinancas.gov.pt:701/sgdtws/documentosTransporte"
	ProductionTransportURL = "https://servicos.portaldasfinancas.gov.pt:401/sgdtws/documentosTransporte"
)

// sgdtNS is the WSDL targetNamespace for documentosTransporte.
const sgdtNS = "https://servicos.portaldasfinancas.gov.pt/sgdtws/documentosTransporte/"

// transportDocRequest is the SOAP body for transport document communication.
//
// Element name and namespace are both load-bearing:
//   - "envioDocumentoTransporteRequestElem" (WSDL xsd:element) — using the
//     complex-type name causes AT to return generic 500 "Internal Error".
//   - prefix xmlns:sgdt rather than default xmlns="..." — WSDL's
//     elementFormDefault is unqualified, so children must NOT inherit the
//     target namespace; default xmlns triggers a "particle 3.1" schema fault.
//
// Field order matches WSDL xs:sequence (CustomerAddress before CustomerName,
// AddressTo before AddressFrom, MovementEndTime before MovementStartTime).
type transportDocRequest struct {
	XMLName               xml.Name         `xml:"sgdt:envioDocumentoTransporteRequestElem"`
	XMLNS                 string           `xml:"xmlns:sgdt,attr"`
	TaxRegistrationNumber string           `xml:"TaxRegistrationNumber"`
	CompanyName           string           `xml:"CompanyName"`
	CompanyAddress        transportAddress `xml:"CompanyAddress"`
	DocumentNumber        string           `xml:"DocumentNumber"`
	ATCUD                 string           `xml:"ATCUD,omitempty"`
	MovementStatus        string           `xml:"MovementStatus"`
	MovementDate          string           `xml:"MovementDate"`
	MovementType          string           `xml:"MovementType"`
	CustomerTaxID         string           `xml:"CustomerTaxID,omitempty"`
	CustomerAddress       transportAddress `xml:"CustomerAddress,omitempty"`
	CustomerName          string           `xml:"CustomerName,omitempty"`
	AddressTo             transportAddress `xml:"AddressTo,omitempty"`
	AddressFrom           transportAddress `xml:"AddressFrom"`
	MovementEndTime       string           `xml:"MovementEndTime,omitempty"`
	MovementStartTime     string           `xml:"MovementStartTime"`
	VehicleID             string           `xml:"VehicleID,omitempty"`
	Lines                 []transportLine  `xml:"Line"`
}

// transportAddress inner fields use omitempty because the WSDL schema types
// (SAFPTtextTypeMandatoryMax*) require minLength=1 — empty string is invalid.
// NOTE: encoding/xml's omitempty does NOT omit empty STRUCT fields, so an
// unpopulated CustomerAddress/AddressTo still emits an empty element (v1
// behaved identically). All current callers populate every address; if AT
// ever rejects an empty element, switch the optional ones to pointers.
type transportAddress struct {
	Addressdetail string `xml:"Addressdetail,omitempty"`
	City          string `xml:"City,omitempty"`
	PostalCode    string `xml:"PostalCode,omitempty"`
	Country       string `xml:"Country,omitempty"`
}

// transportLine: the WSDL Line has only these four fields (plus optional
// OrderReferences) — adding LineNumber or ProductCode causes the gateway to
// return generic 500 "Internal Error".
type transportLine struct {
	ProductDesc   string `xml:"ProductDescription"`
	Quantity      string `xml:"Quantity"`
	UnitOfMeasure string `xml:"UnitOfMeasure,omitempty"`
	UnitPrice     string `xml:"UnitPrice"`
}

// transportDocResponse: per WSDL the response element is
// "envioDocumentoTransporteResponseElem" containing 1..N ResponseStatus
// blocks plus DocumentNumber/ATCUD/ATDocCodeID.
type transportDocResponse struct {
	XMLName        xml.Name                  `xml:"envioDocumentoTransporteResponseElem"`
	ResponseStatus []transportResponseStatus `xml:"ResponseStatus"`
	DocumentNumber string                    `xml:"DocumentNumber,omitempty"`
	ATCUD          string                    `xml:"ATCUD,omitempty"`
	ATDocCodeID    string                    `xml:"ATDocCodeID,omitempty"`
}

type transportResponseStatus struct {
	ReturnCode    int    `xml:"ReturnCode"`
	ReturnMessage string `xml:"ReturnMessage,omitempty"`
}

// TransportResult is AT's answer to a transport-document communication. The
// ATDocCodeID must accompany the goods (printed on the document).
type TransportResult struct {
	ATDocCodeID    string
	DocumentNumber string
	ATCUD          string
	Message        string
}

func addressToTransport(a domain.Address) transportAddress {
	return transportAddress{
		Addressdetail: a.AddressDetail,
		City:          a.City,
		PostalCode:    a.PostalCode,
		Country:       string(a.Country),
	}
}

func shipPointToTransport(sp *domain.ShippingPoint) transportAddress {
	if sp == nil || sp.Address == nil {
		return transportAddress{}
	}
	return addressToTransport(*sp.Address)
}

// xsd:dateTime as UTC ISO 8601 with millisecond precision + Z, matching AT's
// reference Java client (sgdtws emits e.g. 1999-02-01T17:39:36.489Z).
const transportDateTimeFormat = "2006-01-02T15:04:05.000Z"

func buildTransportEnvelope(creds soapCredentials, company domain.Company, mv domain.StockMovement) ([]byte, error) {
	if !mv.DocumentType.IsTransport() {
		return nil, fmt.Errorf("document %s is not a transport document", mv.Number.Format())
	}
	if len(mv.Lines) == 0 {
		return nil, fmt.Errorf("transport document %s has no lines", mv.Number.Format())
	}

	lines := make([]transportLine, len(mv.Lines))
	for i, line := range mv.Lines {
		lines[i] = transportLine{
			ProductDesc:   line.Product.ProductDescription,
			Quantity:      line.Quantity.String(),
			UnitOfMeasure: string(line.Product.Unit),
			UnitPrice:     line.UnitPrice.Format2DP(),
		}
	}

	movementStatus := "N"
	if mv.Status == domain.StatusCancelled {
		movementStatus = "A"
	}

	departure := mv.MovementStartTime.UTC().Format(transportDateTimeFormat)
	var arrival string
	if mv.MovementEndTime != nil {
		arrival = mv.MovementEndTime.UTC().Format(transportDateTimeFormat)
	}

	body := transportDocRequest{
		XMLNS:                 sgdtNS,
		TaxRegistrationNumber: string(company.NIF),
		CompanyName:           company.Name,
		CompanyAddress:        addressToTransport(company.Address),
		DocumentNumber:        mv.Number.Format(),
		ATCUD:                 string(mv.ATCUD),
		MovementStatus:        movementStatus,
		MovementDate:          mv.Date.Format("2006-01-02"),
		MovementType:          mv.DocumentType.String(),
		CustomerTaxID:         string(mv.Customer.CustomerTaxID),
		CustomerAddress:       addressToTransport(mv.Customer.BillingAddress),
		CustomerName:          mv.Customer.CompanyName,
		AddressTo:             shipPointToTransport(mv.ShipTo),
		AddressFrom:           shipPointToTransport(mv.ShipFrom),
		MovementEndTime:       arrival,
		MovementStartTime:     departure,
		Lines:                 lines,
	}
	return buildSOAPEnvelope(creds, body)
}

// CommunicateTransport submits a transport document to AT (sgdtws). Must
// succeed BEFORE goods move; AT returns the ATDocCodeID that has to
// accompany the transport.
func (c *Client) CommunicateTransport(ctx context.Context, company domain.Company, mv domain.StockMovement) (*TransportResult, error) {
	if c.config.TransportURL == "" {
		return nil, fmt.Errorf("at.Config: TransportURL required for CommunicateTransport")
	}
	return soapCall(c, ctx, "CommunicateTransport", c.config.TransportURL,
		func(creds soapCredentials) ([]byte, error) {
			return buildTransportEnvelope(creds, company, mv)
		},
		func(ctx context.Context, resp *transportDocResponse) (*TransportResult, error) {
			var lastMessage string
			for _, status := range resp.ResponseStatus {
				// keep the latest message; AT sends one status block in practice
				lastMessage = status.ReturnMessage
				if status.ReturnCode != 0 {
					return nil, c.atError(ctx, "CommunicateTransport", status.ReturnCode, status.ReturnMessage)
				}
			}
			// Cancellations (MovementStatus=A) void an already-communicated document;
			// AT returns no new ATDocCodeID for them.
			if mv.Status != domain.StatusCancelled && resp.ATDocCodeID == "" {
				return nil, Error{Code: "MISSING_ATDOCCODE", Message: "AT accepted the transport document but returned no ATDocCodeID"}
			}
			return &TransportResult{
				ATDocCodeID:    resp.ATDocCodeID,
				DocumentNumber: resp.DocumentNumber,
				ATCUD:          resp.ATCUD,
				Message:        lastMessage,
			}, nil
		})
}
