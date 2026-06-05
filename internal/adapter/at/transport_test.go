package at

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func testCompany(t *testing.T) domain.Company {
	t.Helper()
	return domain.Company{
		NIF:  domain.TaxID("555555550"),
		Name: "Empresa Teste LDA",
		Address: domain.Address{
			AddressDetail: "Rua Um 1",
			City:          "Lisboa",
			PostalCode:    "1000-001",
			Country:       "PT",
		},
		EACCode: "47190",
	}
}

// testMovement builds a minimal issued GT without going through IssueStockMovement —
// builder input only needs the projected fields.
func testMovement(t *testing.T) domain.StockMovement {
	t.Helper()
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	num, err := domain.NewDocNumber(domain.GT, "S2026", 7)
	if err != nil {
		t.Fatalf("doc number: %v", err)
	}
	mv := domain.StockMovement{}
	mv.Number = num
	mv.ATCUD = domain.ATCUD("AAJFJBB4BD-7")
	mv.Status = domain.StatusNormal
	mv.DocumentType = domain.GT
	mv.Date = start
	mv.Customer = domain.Customer{
		CustomerTaxID: domain.CustomerTaxID("123456789"),
		CompanyName:   "Cliente Exemplo",
		BillingAddress: domain.Address{
			AddressDetail: "Av Dois 2", City: "Porto", PostalCode: "4000-002", Country: "PT",
		},
	}
	mv.Lines = []domain.DocumentLine{{
		Product:   domain.Product{ProductDescription: "Caixa de parafusos"},
		Quantity:  mustQty(t, 2.5),
		UnitPrice: eur(t, 10.50),
	}}
	mv.MovementStartTime = start
	mv.ShipFrom = &domain.ShippingPoint{Address: &domain.Address{
		AddressDetail: "Armazem 1", City: "Lisboa", PostalCode: "1000-001", Country: "PT",
	}}
	mv.ShipTo = &domain.ShippingPoint{Address: &domain.Address{
		AddressDetail: "Loja 9", City: "Porto", PostalCode: "4000-002", Country: "PT",
	}}
	return mv
}

func mustQty(t *testing.T, v float64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(v)
	if err != nil {
		t.Fatalf("quantity: %v", err)
	}
	return q
}

func eur(t *testing.T, v float64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(v)
	if err != nil {
		t.Fatalf("money: %v", err)
	}
	return m
}

func TestTransportEnvelope(t *testing.T) {
	env, err := buildTransportEnvelope(testCreds(), testCompany(t), testMovement(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(env)
	if strings.Contains(s, "xmlns:ser") {
		t.Error("transport envelope must not declare xmlns:ser")
	}
	assertContainsInOrder(t, s,
		`<sgdt:envioDocumentoTransporteRequestElem xmlns:sgdt="https://servicos.portaldasfinancas.gov.pt/sgdtws/documentosTransporte/">`,
		"<TaxRegistrationNumber>555555550</TaxRegistrationNumber>",
		"<CompanyName>Empresa Teste LDA</CompanyName>",
		"<DocumentNumber>GT S2026/7</DocumentNumber>",
		"<ATCUD>AAJFJBB4BD-7</ATCUD>",
		"<MovementStatus>N</MovementStatus>",
		"<MovementDate>2026-07-01</MovementDate>",
		"<MovementType>GT</MovementType>",
		"<CustomerTaxID>123456789</CustomerTaxID>",
		"<CustomerAddress>", // WSDL order: address BEFORE name
		"<CustomerName>Cliente Exemplo</CustomerName>",
		"<AddressTo>",
		"<AddressFrom>",
		"<MovementStartTime>2026-07-01T09:00:00.000Z</MovementStartTime>",
		"<ProductDescription>Caixa de parafusos</ProductDescription>",
		"<Quantity>2.5</Quantity>",
		"<UnitPrice>10.50</UnitPrice>",
	)
	if strings.Contains(s, "<MovementEndTime>") {
		t.Error("unset MovementEndTime must be omitted")
	}
}

func TestTransportEnvelopeCancelledStatus(t *testing.T) {
	mv := testMovement(t)
	mv.Status = domain.StatusCancelled
	env, err := buildTransportEnvelope(testCreds(), testCompany(t), mv)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(env), "<MovementStatus>A</MovementStatus>") {
		t.Error("cancelled movement must map to MovementStatus A")
	}
}

const transportOKResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <env:envioDocumentoTransporteResponseElem xmlns:env="https://servicos.portaldasfinancas.gov.pt/sgdtws/documentosTransporte/">
      <ResponseStatus>
        <ReturnCode>0</ReturnCode>
        <ReturnMessage>OK</ReturnMessage>
      </ResponseStatus>
      <DocumentNumber>GT S2026/7</DocumentNumber>
      <ATCUD>AAJFJBB4BD-7</ATCUD>
      <ATDocCodeID>ATCODETEST123</ATDocCodeID>
    </env:envioDocumentoTransporteResponseElem>
  </S:Body>
</S:Envelope>`

func TestCommunicateTransportSuccess(t *testing.T) {
	var body string
	srv := soapServer(t, transportOKResponse, &body)
	defer srv.Close()

	c := testClientWithURLs(t, srv.URL)
	res, err := c.CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	if err != nil {
		t.Fatalf("CommunicateTransport: %v", err)
	}
	if res.ATDocCodeID != "ATCODETEST123" {
		t.Errorf("ATDocCodeID = %q", res.ATDocCodeID)
	}
	if !strings.Contains(body, "<MovementType>GT</MovementType>") {
		t.Errorf("request body missing movement type:\n%s", body)
	}
}

const transportErrorResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <env:envioDocumentoTransporteResponseElem xmlns:env="https://servicos.portaldasfinancas.gov.pt/sgdtws/documentosTransporte/">
      <ResponseStatus>
        <ReturnCode>33</ReturnCode>
        <ReturnMessage>Documento ja registado</ReturnMessage>
      </ResponseStatus>
    </env:envioDocumentoTransporteResponseElem>
  </S:Body>
</S:Envelope>`

func TestCommunicateTransportATError(t *testing.T) {
	var body string
	srv := soapServer(t, transportErrorResponse, &body)
	defer srv.Close()

	_, err := testClientWithURLs(t, srv.URL).CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "33" {
		t.Fatalf("err = %v, want at.Error 33", err)
	}
}

const transportNoCodeResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <env:envioDocumentoTransporteResponseElem xmlns:env="https://servicos.portaldasfinancas.gov.pt/sgdtws/documentosTransporte/">
      <ResponseStatus><ReturnCode>0</ReturnCode></ResponseStatus>
    </env:envioDocumentoTransporteResponseElem>
  </S:Body>
</S:Envelope>`

func TestCommunicateTransportMissingCode(t *testing.T) {
	var body string
	srv := soapServer(t, transportNoCodeResponse, &body)
	defer srv.Close()

	_, err := testClientWithURLs(t, srv.URL).CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "MISSING_ATDOCCODE" {
		t.Fatalf("err = %v, want MISSING_ATDOCCODE", err)
	}
}

func TestCommunicateTransportCancellationSuccess(t *testing.T) {
	var body string
	srv := soapServer(t, transportNoCodeResponse, &body)
	defer srv.Close()
	mv := testMovement(t)
	mv.Status = domain.StatusCancelled
	res, err := testClientWithURLs(t, srv.URL).CommunicateTransport(context.Background(), testCompany(t), mv)
	if err != nil {
		t.Fatalf("cancellation communication: %v", err)
	}
	if res.ATDocCodeID != "" {
		t.Errorf("ATDocCodeID = %q, want empty for cancellation", res.ATDocCodeID)
	}
	if !strings.Contains(body, "<MovementStatus>A</MovementStatus>") {
		t.Errorf("request must carry MovementStatus A:\n%s", body)
	}
}

func TestTransportEnvelopeWithEndTime(t *testing.T) {
	mv := testMovement(t)
	end := mv.MovementStartTime.Add(2 * time.Hour)
	mv.MovementEndTime = &end
	env, err := buildTransportEnvelope(testCreds(), testCompany(t), mv)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(env), "<MovementEndTime>2026-07-01T11:00:00.000Z</MovementEndTime>") {
		t.Errorf("missing formatted MovementEndTime:\n%s", env)
	}
}

func TestTransportEnvelopeRejectsNoLines(t *testing.T) {
	mv := testMovement(t)
	mv.Lines = nil
	if _, err := buildTransportEnvelope(testCreds(), testCompany(t), mv); err == nil {
		t.Fatal("want error for transport document without lines")
	}
}

// testClientWithURLs builds a client whose series/transport/invoice URLs all
// point at the same httptest server.
func testClientWithURLs(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Config{
		SeriesURL: url, TransportURL: url, InvoiceURL: url,
		TaxpayerNIF: "555555550", Username: "1", Password: "secret",
		SoftwareCertNum: "0",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}
