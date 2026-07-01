package at

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// formatUsername returns the WS-Security username in AT's required form
// "<NIF>/<UserId>". Accepts either a bare UserId or a full "NIF/UserId" — if
// the caller already passed a "/", treat it as fully qualified and avoid
// prefixing the NIF a second time.
func formatUsername(nif, username string) string {
	if strings.Contains(username, "/") {
		return username
	}
	return fmt.Sprintf("%s/%s", nif, username)
}

const (
	soapEnvNS = "http://schemas.xmlsoap.org/soap/envelope/"
	// AT mandates the older 2002/12 secext namespace per its
	// "Comunicação de Séries — Aspetos Genéricos" manual, not the OASIS WSS
	// 1.0 namespace. Using the OASIS URI causes the AT XSD validator to
	// reject the request with a generic "Erro - Pedido do Cliente".
	wsseNS = "http://schemas.xmlsoap.org/ws/2002/12/secext"
	// SeriesWS targetNamespace per the official WSDL (SeriesWS.wsdl).
	// The trailing slash is required: "http://at.gov.pt/series" causes the AT
	// XSD validator to reject the request with "Erro - Pedido do Cliente".
	seriesNS = "http://at.gov.pt/"
)

// soapEnvelope is the root SOAP envelope structure.
type soapEnvelope struct {
	XMLName xml.Name   `xml:"soapenv:Envelope"`
	SoapNS  string     `xml:"xmlns:soapenv,attr"`
	WsseNS  string     `xml:"xmlns:wsse,attr"`
	SerNS   string     `xml:"xmlns:ser,attr,omitempty"`
	Header  soapHeader `xml:"soapenv:Header"`
	Body    soapBody   `xml:"soapenv:Body"`
}

type soapHeader struct {
	Security wsseSecurity `xml:"wsse:Security"`
}

type wsseSecurity struct {
	UsernameToken wsseUsernameToken `xml:"wsse:UsernameToken"`
}

type wsseUsernameToken struct {
	Username string `xml:"wsse:Username"`
	Password string `xml:"wsse:Password"`
	Nonce    string `xml:"wsse:Nonce,omitempty"`
	Created  string `xml:"wsse:Created,omitempty"`
}

// soapCredentials bundles the authentication fields for envelope construction.
// When Nonce and Created are empty, plain-text authentication is used
// (test environment only).
type soapCredentials struct {
	NIF      string
	Username string
	Password string // Plain-text or AES-encrypted (base64)
	Nonce    string // RSA-encrypted AES key (base64); empty for plain-text mode
	Created  string // AES-encrypted timestamp (base64); empty for plain-text mode
	// Fatcorews-specific software identity (RegisterInvoice payload).
	TaxEntity       string
	SoftwareCertNum int
}

type soapBody struct {
	Content any `xml:",any"`
}

type soapFault struct {
	XMLName     xml.Name `xml:"Fault"`
	FaultCode   string   `xml:"faultcode"`
	FaultString string   `xml:"faultstring"`
	Detail      string   `xml:"detail,omitempty"`
}

type soapResponseEnvelope struct {
	XMLName xml.Name         `xml:"Envelope"`
	Body    soapResponseBody `xml:"Body"`
}

type soapResponseBody struct {
	Fault    *soapFault `xml:"Fault,omitempty"`
	InnerXML []byte     `xml:",innerxml"`
}

// ============================================================================
// SERIESWS OPERATION TYPES (field order = WSDL xs:sequence; AT validates it)
// ============================================================================

type seriesRegistrationRequest struct {
	XMLName            xml.Name `xml:"ser:registarSerie"`
	Serie              string   `xml:"serie"`
	TipoSerie          string   `xml:"tipoSerie"`
	ClasseDoc          string   `xml:"classeDoc"`
	TipoDoc            string   `xml:"tipoDoc"`
	NumInicialSeq      int      `xml:"numInicialSeq"`
	DataInicioPrevUtil string   `xml:"dataInicioPrevUtiliz"`
	NumCertSWFatur     string   `xml:"numCertSWFatur"`
	MeioProcessamento  string   `xml:"meioProcessamento"`
}

type seriesRegistrationResponse struct {
	XMLName xml.Name   `xml:"registarSerieResponse"`
	Resp    seriesResp `xml:"registarSerieResp"`
}

// seriesResp mirrors the WSDL seriesResp complex type shared by the
// registar/finalizar/anular responses.
type seriesResp struct {
	InfoSerie      *atSeriesInfo     `xml:"infoSerie,omitempty"`
	InfoResultOper atOperationResult `xml:"infoResultOper"`
}

type seriesFinalizationRequest struct {
	XMLName             xml.Name `xml:"ser:finalizarSerie"`
	Serie               string   `xml:"serie"`
	ClasseDoc           string   `xml:"classeDoc"`
	TipoDoc             string   `xml:"tipoDoc"`
	CodValidacaoSerie   string   `xml:"codValidacaoSerie"`
	SeqUltimoDocEmitido int      `xml:"seqUltimoDocEmitido"`
	Justificacao        string   `xml:"justificacao,omitempty"`
}

type seriesFinalizationResponse struct {
	XMLName xml.Name   `xml:"finalizarSerieResponse"`
	Resp    seriesResp `xml:"finalizarSerieResp"`
}

// seriesCancellationRequest is the anularSerie request per SeriesWS.wsdl:
// serie, classeDoc, tipoDoc, codValidacaoSerie, motivo, declaracaoNaoEmissao.
// declaracaoNaoEmissao must be true — the taxpayer declares no documents were
// ever issued in the series; AT rejects the call otherwise.
type seriesCancellationRequest struct {
	XMLName              xml.Name `xml:"ser:anularSerie"`
	Serie                string   `xml:"serie"`
	ClasseDoc            string   `xml:"classeDoc"`
	TipoDoc              string   `xml:"tipoDoc"`
	CodValidacaoSerie    string   `xml:"codValidacaoSerie"`
	Motivo               string   `xml:"motivo"`
	DeclaracaoNaoEmissao bool     `xml:"declaracaoNaoEmissao"`
}

type seriesCancellationResponse struct {
	XMLName xml.Name   `xml:"anularSerieResponse"`
	Resp    seriesResp `xml:"anularSerieResp"`
}

type seriesStatusRequest struct {
	XMLName   xml.Name `xml:"ser:consultarSeries"`
	Serie     string   `xml:"serie"`
	ClasseDoc string   `xml:"classeDoc"`
	TipoDoc   string   `xml:"tipoDoc"`
}

type seriesStatusResponse struct {
	XMLName xml.Name         `xml:"consultarSeriesResponse"`
	Resp    seriesStatusResp `xml:"consultarSeriesResp"`
}

type seriesStatusResp struct {
	InfoSerie      []atSeriesInfo    `xml:"infoSerie"`
	InfoResultOper atOperationResult `xml:"infoResultOper"`
}

// atSeriesInfo mirrors the WSDL seriesInfo complex type returned by all
// SeriesWS operations.
type atSeriesInfo struct {
	Serie                string `xml:"serie"`
	TipoSerie            string `xml:"tipoSerie"`
	ClasseDoc            string `xml:"classeDoc"`
	TipoDoc              string `xml:"tipoDoc"`
	NumInicialSeq        int    `xml:"numInicialSeq"`
	NumFinalSeq          int    `xml:"numFinalSeq,omitempty"`
	DataInicioPrevUtiliz string `xml:"dataInicioPrevUtiliz"`
	SeqUltimoDocEmitido  int    `xml:"seqUltimoDocEmitido,omitempty"`
	MeioProcessamento    string `xml:"meioProcessamento"`
	NumCertSWFatur       int    `xml:"numCertSWFatur"`
	CodValidacaoSerie    string `xml:"codValidacaoSerie"`
	DataRegisto          string `xml:"dataRegisto"`
	Estado               string `xml:"estado"`
	MotivoEstado         string `xml:"motivoEstado,omitempty"`
	Justificacao         string `xml:"justificacao,omitempty"`
	DataEstado           string `xml:"dataEstado"`
	NifComunicou         string `xml:"nifComunicou"`
}

// atOperationResult mirrors the WSDL operationResultInfo complex type.
// AT uses 2xxx codes for success/info (live-observed: 2001 registered ok,
// 2002 search ok, 2003 cancelled ok) and 3xxx+ for errors.
type atOperationResult struct {
	CodResultOper int    `xml:"codResultOper"`
	MsgResultOper string `xml:"msgResultOper"`
}

const atErrorCodeMin = 3000

// IsError reports whether the AT operation code signals failure.
func (r atOperationResult) IsError() bool {
	return r.CodResultOper >= atErrorCodeMin
}

// ============================================================================
// ENVELOPE BUILD / RESPONSE PARSE
// ============================================================================

// buildSOAPEnvelope creates a SOAP envelope with WS-Security headers.
func buildSOAPEnvelope(creds soapCredentials, body any) ([]byte, error) {
	// The xmlns:ser declaration is only included for SeriesWS bodies (which
	// use the ser: prefix); sgdtws/fatcorews bodies declare their own xmlns
	// inline, and some AT services reject unused namespace declarations on
	// the envelope. The case list below is the complete SeriesWS operation set
	// per the WSDL — extend it only if AT adds operations.
	var serNS string
	switch body.(type) {
	case seriesRegistrationRequest, seriesFinalizationRequest, seriesCancellationRequest, seriesStatusRequest:
		serNS = seriesNS
	}

	env := soapEnvelope{
		SoapNS: soapEnvNS,
		WsseNS: wsseNS,
		SerNS:  serNS,
		Header: soapHeader{
			Security: wsseSecurity{
				UsernameToken: wsseUsernameToken{
					Username: formatUsername(creds.NIF, creds.Username),
					Password: creds.Password,
					Nonce:    creds.Nonce,
					Created:  creds.Created,
				},
			},
		},
		Body: soapBody{Content: body},
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)

	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(env); err != nil {
		return nil, fmt.Errorf("encoding SOAP envelope: %w", err)
	}
	return buf.Bytes(), nil
}

// parseSOAPResponse extracts the body content from a SOAP response.
func parseSOAPResponse(data []byte, result any) error {
	var env soapResponseEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("parsing SOAP response: %w", err)
	}
	if env.Body.Fault != nil {
		return Error{Code: env.Body.Fault.FaultCode, Message: env.Body.Fault.FaultString}
	}
	if result != nil && len(env.Body.InnerXML) > 0 {
		if err := xml.Unmarshal(env.Body.InnerXML, result); err != nil {
			return fmt.Errorf("parsing SOAP body content: %w", err)
		}
	}
	return nil
}

// ============================================================================
// PER-OPERATION ENVELOPE BUILDERS
// ============================================================================

func buildSeriesRegistrationEnvelope(creds soapCredentials, req SeriesRegistration, certNum string) ([]byte, error) {
	class, err := docClass(req.DocType)
	if err != nil {
		return nil, err
	}
	return buildSOAPEnvelope(creds, seriesRegistrationRequest{
		Serie:              req.SeriesID,
		TipoSerie:          req.SeriesType,
		ClasseDoc:          class,
		TipoDoc:            req.DocType.String(),
		NumInicialSeq:      req.InitialSeq,
		DataInicioPrevUtil: req.ExpectedStartDate.Format("2006-01-02"),
		NumCertSWFatur:     certNum,
		MeioProcessamento:  "PI", // documents produced by this invoicing software
	})
}

func buildSeriesFinalizationEnvelope(creds soapCredentials, req SeriesFinalization) ([]byte, error) {
	class, err := docClass(req.DocType)
	if err != nil {
		return nil, err
	}
	return buildSOAPEnvelope(creds, seriesFinalizationRequest{
		Serie:               req.SeriesID,
		ClasseDoc:           class,
		TipoDoc:             req.DocType.String(),
		CodValidacaoSerie:   req.ATCode,
		SeqUltimoDocEmitido: req.LastSeq,
		Justificacao:        req.Justification,
	})
}

func buildSeriesCancellationEnvelope(creds soapCredentials, req SeriesCancellation) ([]byte, error) {
	class, err := docClass(req.DocType)
	if err != nil {
		return nil, err
	}
	return buildSOAPEnvelope(creds, seriesCancellationRequest{
		Serie:                req.SeriesID,
		ClasseDoc:            class,
		TipoDoc:              req.DocType.String(),
		CodValidacaoSerie:    req.ATCode,
		Motivo:               req.Reason,
		DeclaracaoNaoEmissao: true,
	})
}

func buildSeriesStatusEnvelope(creds soapCredentials, seriesID string, docType domain.DocumentType) ([]byte, error) {
	class, err := docClass(docType)
	if err != nil {
		return nil, err
	}
	return buildSOAPEnvelope(creds, seriesStatusRequest{
		Serie:     seriesID,
		ClasseDoc: class,
		TipoDoc:   docType.String(),
	})
}
