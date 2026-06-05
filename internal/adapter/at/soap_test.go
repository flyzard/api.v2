package at

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func testCreds() soapCredentials {
	return soapCredentials{NIF: "555555550", Username: "1", Password: "secret"}
}

// assertContainsInOrder checks all needles appear in haystack in order —
// AT validates element order against the WSDL xs:sequence.
func assertContainsInOrder(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	pos := 0
	for _, n := range needles {
		i := strings.Index(haystack[pos:], n)
		if i < 0 {
			t.Fatalf("missing or out of order: %q\nin:\n%s", n, haystack)
		}
		pos += i + len(n)
	}
}

func TestRegistrationEnvelope(t *testing.T) {
	env, err := buildSeriesRegistrationEnvelope(testCreds(), SeriesRegistration{
		SeriesID:          "S2026",
		DocType:           domain.FT,
		SeriesType:        "N",
		InitialSeq:        1,
		ExpectedStartDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}, "9999")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(env)
	assertContainsInOrder(t, s,
		`xmlns:ser="http://at.gov.pt/"`,
		"<wsse:Username>555555550/1</wsse:Username>",
		"<wsse:Password>secret</wsse:Password>",
		"<ser:registarSerie>",
		"<serie>S2026</serie>",
		"<tipoSerie>N</tipoSerie>",
		"<classeDoc>SI</classeDoc>",
		"<tipoDoc>FT</tipoDoc>",
		"<numInicialSeq>1</numInicialSeq>",
		"<dataInicioPrevUtiliz>2026-07-01</dataInicioPrevUtiliz>",
		"<numCertSWFatur>9999</numCertSWFatur>",
		"<meioProcessamento>PI</meioProcessamento>",
	)
}

func TestFinalizationEnvelope(t *testing.T) {
	env, err := buildSeriesFinalizationEnvelope(testCreds(), SeriesFinalization{
		SeriesID: "S2026", DocType: domain.FT, ATCode: "BCDFGH37", LastSeq: 42, Justification: "done",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	assertContainsInOrder(t, string(env),
		"<ser:finalizarSerie>",
		"<serie>S2026</serie>",
		"<classeDoc>SI</classeDoc>",
		"<tipoDoc>FT</tipoDoc>",
		"<codValidacaoSerie>BCDFGH37</codValidacaoSerie>",
		"<seqUltimoDocEmitido>42</seqUltimoDocEmitido>",
		"<justificacao>done</justificacao>",
	)
}

func TestCancellationEnvelope(t *testing.T) {
	env, err := buildSeriesCancellationEnvelope(testCreds(), SeriesCancellation{
		SeriesID: "S2026", DocType: domain.RC, ATCode: "BCDFGH48", Reason: CancelReasonError,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	assertContainsInOrder(t, string(env),
		"<ser:anularSerie>",
		"<serie>S2026</serie>",
		"<classeDoc>PY</classeDoc>",
		"<tipoDoc>RC</tipoDoc>",
		"<codValidacaoSerie>BCDFGH48</codValidacaoSerie>",
		"<motivo>ER</motivo>",
		"<declaracaoNaoEmissao>true</declaracaoNaoEmissao>",
	)
}

func TestStatusEnvelope(t *testing.T) {
	env, err := buildSeriesStatusEnvelope(testCreds(), "S2026", domain.GT)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	assertContainsInOrder(t, string(env),
		"<ser:consultarSeries>",
		"<serie>S2026</serie>",
		"<classeDoc>MG</classeDoc>",
		"<tipoDoc>GT</tipoDoc>",
	)
}

func TestEnvelopeRejectsUnknownDocClass(t *testing.T) {
	_, err := buildSeriesStatusEnvelope(testCreds(), "S2026", domain.DocumentType("XX"))
	if err == nil {
		t.Fatal("want doc-class error")
	}
}

func TestEnvelopeOmitsSerNamespaceForNonSeriesBodies(t *testing.T) {
	type otherBody struct {
		XMLName xml.Name `xml:"other:Op"`
		V       string   `xml:"V"`
	}
	env, err := buildSOAPEnvelope(testCreds(), otherBody{V: "x"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if strings.Contains(string(env), "xmlns:ser") {
		t.Fatalf("non-series body must not declare xmlns:ser:\n%s", env)
	}
}

func TestEnvelopeKeepsSerNamespaceForSeriesBodies(t *testing.T) {
	env, err := buildSeriesStatusEnvelope(testCreds(), "S2026", domain.FT)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(env), `xmlns:ser="http://at.gov.pt/"`) {
		t.Fatalf("series body must declare xmlns:ser:\n%s", env)
	}
}

func TestParseSOAPFault(t *testing.T) {
	fault := `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <S:Fault>
      <faultcode>S:Client</faultcode>
      <faultstring>Erro - Pedido do Cliente</faultstring>
    </S:Fault>
  </S:Body>
</S:Envelope>`
	err := parseSOAPResponse([]byte(fault), nil)
	atErr, ok := err.(Error)
	if !ok {
		t.Fatalf("err = %T %v, want at.Error", err, err)
	}
	if atErr.Message != "Erro - Pedido do Cliente" {
		t.Errorf("Message = %q", atErr.Message)
	}
}

func TestParseRegistrationResponse(t *testing.T) {
	body := `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <ns2:registarSerieResponse xmlns:ns2="http://at.gov.pt/">
      <registarSerieResp>
        <infoSerie>
          <serie>S2026</serie>
          <tipoSerie>N</tipoSerie>
          <classeDoc>SI</classeDoc>
          <tipoDoc>FT</tipoDoc>
          <numInicialSeq>1</numInicialSeq>
          <dataInicioPrevUtiliz>2026-07-01</dataInicioPrevUtiliz>
          <meioProcessamento>PI</meioProcessamento>
          <numCertSWFatur>9999</numCertSWFatur>
          <codValidacaoSerie>BCDFGH37</codValidacaoSerie>
          <dataRegisto>2026-06-04</dataRegisto>
          <estado>A</estado>
          <dataEstado>2026-06-04</dataEstado>
          <nifComunicou>555555550</nifComunicou>
        </infoSerie>
        <infoResultOper>
          <codResultOper>2001</codResultOper>
          <msgResultOper>Serie registada com sucesso.</msgResultOper>
        </infoResultOper>
      </registarSerieResp>
    </ns2:registarSerieResponse>
  </S:Body>
</S:Envelope>`
	var resp seriesRegistrationResponse
	if err := parseSOAPResponse([]byte(body), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Resp.InfoResultOper.IsError() {
		t.Fatal("2001 must not be an error")
	}
	if resp.Resp.InfoSerie == nil || resp.Resp.InfoSerie.CodValidacaoSerie != "BCDFGH37" {
		t.Fatalf("infoSerie = %+v", resp.Resp.InfoSerie)
	}
}
