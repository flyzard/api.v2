package at

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// testInvoice builds a minimal issued FT with two NOR-rate lines (same tax
// bucket — must collapse into ONE LineSummary) plus one exempt line.
func testInvoice(t *testing.T) domain.SalesInvoice {
	t.Helper()
	date := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	num, err := domain.NewDocNumber(domain.FT, "S2026", 3)
	if err != nil {
		t.Fatalf("doc number: %v", err)
	}
	norRate, err := domain.GetTaxRate(domain.PT, domain.TaxNormal, "")
	if err != nil {
		t.Fatalf("nor rate: %v", err)
	}
	exemptRate, err := domain.GetTaxRate(domain.PT, domain.TaxExempt, domain.M05)
	if err != nil {
		t.Fatalf("exempt rate: %v", err)
	}

	inv := domain.SalesInvoice{}
	inv.Number = num
	inv.ATCUD = domain.ATCUD("AAJFJBB4BD-3")
	inv.Hash = domain.Hash("Axxxxxxxxx" + "Bxxxxxxxxx" + "Cxxxxxxxxx" + "Dxxxxxxxxx")
	inv.Status = domain.StatusNormal
	inv.StatusDate = date
	inv.SystemEntryDate = date
	inv.DocumentType = domain.FT
	inv.Date = date
	inv.Customer = domain.Customer{
		CustomerTaxID:  domain.CustomerTaxID("123456789"),
		CompanyName:    "Cliente Exemplo",
		BillingAddress: domain.Address{AddressDetail: "Av Dois 2", City: "Porto", PostalCode: "4000-002", Country: "PT"},
	}
	inv.Lines = []domain.DocumentLine{
		{Quantity: mustQty(t, 1), UnitPrice: eur(t, 10), TaxPointDate: date,
			Tax: domain.VATTax{Rate: norRate}},
		{Quantity: mustQty(t, 1), UnitPrice: eur(t, 20), TaxPointDate: date,
			Tax: domain.VATTax{Rate: norRate}},
		{Quantity: mustQty(t, 1), UnitPrice: eur(t, 5), TaxPointDate: date,
			Tax: domain.VATTax{Rate: exemptRate, ExemptReason: "Artigo 14.o do CIVA"}},
	}
	inv.Totals = domain.Totals{
		NetTotal:   eur(t, 35),
		TaxTotal:   eur(t, 6.90), // 30.00 * 23%
		GrossTotal: eur(t, 41.90),
	}
	return inv
}

func TestInvoiceEnvelopeGroupsLines(t *testing.T) {
	creds := testCreds()
	creds.SoftwareCertNum = 9999
	creds.TaxEntity = "Global"
	env, err := buildInvoiceEnvelope(creds, testCompany(t), testInvoice(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(env)
	if strings.Contains(s, "xmlns:ser") {
		t.Error("fatcorews envelope must not declare xmlns:ser")
	}
	if got := strings.Count(s, "<doc:LineSummary>"); got != 2 {
		t.Errorf("LineSummary count = %d, want 2 (two NOR lines collapse, exempt separate)", got)
	}
	assertContainsInOrder(t, s,
		`<doc:RegisterInvoiceRequest xmlns:doc="http://factemi.at.min_financas.pt/documents">`,
		"<doc:eFaturaMDVersion>0.0.1</doc:eFaturaMDVersion>",
		"<doc:AuditFileVersion>1.04_01</doc:AuditFileVersion>",
		"<doc:TaxRegistrationNumber>555555550</doc:TaxRegistrationNumber>",
		"<doc:TaxEntity>Global</doc:TaxEntity>",
		"<doc:SoftwareCertificateNumber>9999</doc:SoftwareCertificateNumber>",
		"<doc:InvoiceNo>FT S2026/3</doc:InvoiceNo>",
		"<doc:ATCUD>AAJFJBB4BD-3</doc:ATCUD>",
		"<doc:InvoiceType>FT</doc:InvoiceType>",
		"<doc:CustomerTaxID>123456789</doc:CustomerTaxID>",
		"<doc:InvoiceStatus>N</doc:InvoiceStatus>",
		"<doc:HashCharacters>ABCD</doc:HashCharacters>",
		"<doc:EACCode>47190</doc:EACCode>",
		"<doc:DebitCreditIndicator>C</doc:DebitCreditIndicator>",
		"<doc:Amount>30.00</doc:Amount>",
		"<doc:TaxPercentage>23.00</doc:TaxPercentage>",
		"<doc:TaxExemptionCode>M05</doc:TaxExemptionCode>",
		"<doc:TaxPayable>6.90</doc:TaxPayable>",
		"<doc:NetTotal>35.00</doc:NetTotal>",
		"<doc:GrossTotal>41.90</doc:GrossTotal>",
	)
}

func TestInvoiceEnvelopeUncertifiedHashChars(t *testing.T) {
	env, err := buildInvoiceEnvelope(testCreds(), testCompany(t), testInvoice(t)) // certNum 0
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(env), "<doc:HashCharacters>0</doc:HashCharacters>") {
		t.Error("uncertified software must send HashCharacters=0")
	}
}

func TestInvoiceEnvelopeCreditNoteDebits(t *testing.T) {
	inv := testInvoice(t)
	inv.DocumentType = domain.NC
	env, err := buildInvoiceEnvelope(testCreds(), testCompany(t), inv)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(env), "<doc:DebitCreditIndicator>D</doc:DebitCreditIndicator>") {
		t.Error("NC must use DebitCreditIndicator D")
	}
}

func TestInvoiceEnvelopeNotSubjectLine(t *testing.T) {
	inv := testInvoice(t)
	inv.Lines = []domain.DocumentLine{{
		Quantity: mustQty(t, 1), UnitPrice: eur(t, 10), TaxPointDate: inv.Date,
		Tax: domain.NotSubjectTax{Jurisdiction: "PT", Reason: domain.M99, ReasonText: "Nao sujeito a IVA"},
	}}
	env, err := buildInvoiceEnvelope(testCreds(), testCompany(t), inv)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	assertContainsInOrder(t, string(env),
		"<doc:TaxType>NS</doc:TaxType>",
		"<doc:TaxCode>M99</doc:TaxCode>",
		"<doc:TaxPercentage>0.00</doc:TaxPercentage>",
		"<doc:TaxExemptionCode>M99</doc:TaxExemptionCode>",
	)
}

func TestInvoiceEnvelopeNilTaxRejected(t *testing.T) {
	inv := testInvoice(t)
	inv.Lines = []domain.DocumentLine{{Quantity: mustQty(t, 1), UnitPrice: eur(t, 10), TaxPointDate: inv.Date, Tax: nil}}
	if _, err := buildInvoiceEnvelope(testCreds(), testCompany(t), inv); err == nil {
		t.Fatal("want error for line with nil tax")
	}
}

const invoiceOKResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <RegisterInvoiceResponse xmlns="http://factemi.at.min_financas.pt/documents">
      <Response>
        <CodigoResposta>0</CodigoResposta>
        <Mensagem>Documento registado com sucesso.</Mensagem>
        <DataOperacao>2026-07-01T10:00:05</DataOperacao>
      </Response>
    </RegisterInvoiceResponse>
  </S:Body>
</S:Envelope>`

func TestCommunicateInvoiceSuccess(t *testing.T) {
	var body string
	srv := soapServer(t, invoiceOKResponse, &body)
	defer srv.Close()

	res, err := testClientWithURLs(t, srv.URL).CommunicateInvoice(context.Background(), testCompany(t), testInvoice(t))
	if err != nil {
		t.Fatalf("CommunicateInvoice: %v", err)
	}
	if res.Code != 0 || res.Message == "" {
		t.Errorf("result = %+v", res)
	}
	if !strings.Contains(body, "<doc:InvoiceNo>FT S2026/3</doc:InvoiceNo>") {
		t.Errorf("request body missing invoice no:\n%s", body)
	}
}

const invoiceErrorResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <RegisterInvoiceResponse xmlns="http://factemi.at.min_financas.pt/documents">
      <Response>
        <CodigoResposta>-3</CodigoResposta>
        <Mensagem>Erro de validacao.</Mensagem>
      </Response>
    </RegisterInvoiceResponse>
  </S:Body>
</S:Envelope>`

func TestCommunicateInvoiceATError(t *testing.T) {
	var body string
	srv := soapServer(t, invoiceErrorResponse, &body)
	defer srv.Close()

	_, err := testClientWithURLs(t, srv.URL).CommunicateInvoice(context.Background(), testCompany(t), testInvoice(t))
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "-3" {
		t.Fatalf("err = %v, want at.Error -3", err)
	}
}
