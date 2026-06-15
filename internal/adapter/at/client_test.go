package at

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// soapServer returns an httptest server that records the request body and
// answers with the given SOAP response.
func soapServer(t *testing.T, response string, gotBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*gotBody = string(b)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(response))
	}))
}

func testClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Config{
		SeriesURL:       url,
		TaxpayerNIF:     "555555550",
		Username:        "1",
		Password:        "secret",
		SoftwareCertNum: "9999",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

const registerOKResponse = `<?xml version="1.0"?>
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

func TestRegisterSeriesSuccess(t *testing.T) {
	var body string
	srv := soapServer(t, registerOKResponse, &body)
	defer srv.Close()

	res, err := testClient(t, srv.URL).RegisterSeries(context.Background(), SeriesRegistration{
		SeriesID: "S2026", DocType: domain.FT, SeriesType: "N", InitialSeq: 1,
		ExpectedStartDate: atT0,
	})
	if err != nil {
		t.Fatalf("RegisterSeries: %v", err)
	}
	if res.ValidationCode != "BCDFGH37" {
		t.Errorf("ValidationCode = %q", res.ValidationCode)
	}
	if res.Status != domain.SeriesActive {
		t.Errorf("Status = %s", res.Status)
	}
	if !strings.Contains(body, "<ser:registarSerie>") || !strings.Contains(body, "<numCertSWFatur>9999</numCertSWFatur>") {
		t.Errorf("request body missing expected elements:\n%s", body)
	}
}

const atErrorResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <ns2:registarSerieResponse xmlns:ns2="http://at.gov.pt/">
      <registarSerieResp>
        <infoResultOper>
          <codResultOper>4001</codResultOper>
          <msgResultOper>Serie ja registada.</msgResultOper>
        </infoResultOper>
      </registarSerieResp>
    </ns2:registarSerieResponse>
  </S:Body>
</S:Envelope>`

func TestRegisterSeriesATError(t *testing.T) {
	var body string
	srv := soapServer(t, atErrorResponse, &body)
	defer srv.Close()

	_, err := testClient(t, srv.URL).RegisterSeries(context.Background(), SeriesRegistration{
		SeriesID: "S2026", DocType: domain.FT, SeriesType: "N", InitialSeq: 1, ExpectedStartDate: atT0,
	})
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "4001" {
		t.Fatalf("err = %v, want at.Error 4001", err)
	}
}

const finalizeOKResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <ns2:finalizarSerieResponse xmlns:ns2="http://at.gov.pt/">
      <finalizarSerieResp>
        <infoResultOper>
          <codResultOper>2003</codResultOper>
          <msgResultOper>Serie finalizada com sucesso.</msgResultOper>
        </infoResultOper>
      </finalizarSerieResp>
    </ns2:finalizarSerieResponse>
  </S:Body>
</S:Envelope>`

func TestFinalizeSeries(t *testing.T) {
	var body string
	srv := soapServer(t, finalizeOKResponse, &body)
	defer srv.Close()

	err := testClient(t, srv.URL).FinalizeSeries(context.Background(), SeriesFinalization{
		SeriesID: "S2026", DocType: domain.FT, ATCode: "BCDFGH37", LastSeq: 42,
	})
	if err != nil {
		t.Fatalf("FinalizeSeries: %v", err)
	}
	if !strings.Contains(body, "<seqUltimoDocEmitido>42</seqUltimoDocEmitido>") {
		t.Errorf("request body missing last seq:\n%s", body)
	}
}

const cancelOKResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <ns2:anularSerieResponse xmlns:ns2="http://at.gov.pt/">
      <anularSerieResp>
        <infoResultOper>
          <codResultOper>2004</codResultOper>
          <msgResultOper>Serie anulada com sucesso.</msgResultOper>
        </infoResultOper>
      </anularSerieResp>
    </ns2:anularSerieResponse>
  </S:Body>
</S:Envelope>`

func TestCancelSeries(t *testing.T) {
	var body string
	srv := soapServer(t, cancelOKResponse, &body)
	defer srv.Close()

	err := testClient(t, srv.URL).CancelSeries(context.Background(), SeriesCancellation{
		SeriesID: "S2026", DocType: domain.FT, ATCode: "BCDFGH37", Reason: CancelReasonError,
	})
	if err != nil {
		t.Fatalf("CancelSeries: %v", err)
	}
	if !strings.Contains(body, "<declaracaoNaoEmissao>true</declaracaoNaoEmissao>") {
		t.Errorf("request body missing declaracaoNaoEmissao:\n%s", body)
	}
}

const statusOKResponse = `<?xml version="1.0"?>
<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/">
  <S:Body>
    <ns2:consultarSeriesResponse xmlns:ns2="http://at.gov.pt/">
      <consultarSeriesResp>
        <infoSerie>
          <serie>S2026</serie>
          <tipoSerie>N</tipoSerie>
          <classeDoc>SI</classeDoc>
          <tipoDoc>FT</tipoDoc>
          <numInicialSeq>1</numInicialSeq>
          <dataInicioPrevUtiliz>2026-07-01</dataInicioPrevUtiliz>
          <seqUltimoDocEmitido>17</seqUltimoDocEmitido>
          <meioProcessamento>PI</meioProcessamento>
          <numCertSWFatur>9999</numCertSWFatur>
          <codValidacaoSerie>BCDFGH37</codValidacaoSerie>
          <dataRegisto>2026-06-04</dataRegisto>
          <estado>A</estado>
          <dataEstado>2026-06-04</dataEstado>
          <nifComunicou>555555550</nifComunicou>
        </infoSerie>
        <infoResultOper>
          <codResultOper>2002</codResultOper>
          <msgResultOper>OK</msgResultOper>
        </infoResultOper>
      </consultarSeriesResp>
    </ns2:consultarSeriesResponse>
  </S:Body>
</S:Envelope>`

func TestGetSeriesStatus(t *testing.T) {
	var body string
	srv := soapServer(t, statusOKResponse, &body)
	defer srv.Close()

	st, err := testClient(t, srv.URL).GetSeriesStatus(context.Background(), "S2026", domain.FT)
	if err != nil {
		t.Fatalf("GetSeriesStatus: %v", err)
	}
	if st.LastSeq != 17 || st.Status != domain.SeriesActive || st.ValidationCode != "BCDFGH37" {
		t.Errorf("status = %+v", st)
	}
}

// TestRegisterSeriesReconcilesAlreadyRegistered pins the lost-response
// recovery: registarSerie is not idempotent, so a committed-but-unacknowledged
// registration makes the retry fail with "série já registada" (4xxx). The
// client must then consult the series and, finding it registered, return its
// state as success — mirroring NullClient's idempotent RegisterSeries.
func TestRegisterSeriesReconcilesAlreadyRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if strings.Contains(string(b), "consultarSeries") {
			_, _ = w.Write([]byte(statusOKResponse))
			return
		}
		_, _ = w.Write([]byte(atErrorResponse)) // 4001 "Serie ja registada."
	}))
	defer srv.Close()

	res, err := testClient(t, srv.URL).RegisterSeries(context.Background(), SeriesRegistration{
		SeriesID: "S2026", DocType: domain.FT, SeriesType: "N", InitialSeq: 1, ExpectedStartDate: atT0,
	})
	if err != nil {
		t.Fatalf("RegisterSeries: %v, want reconciled success", err)
	}
	if res.ValidationCode != "BCDFGH37" {
		t.Errorf("ValidationCode = %q, want BCDFGH37 (recovered from consultarSeries)", res.ValidationCode)
	}
	if res.Status != domain.SeriesActive {
		t.Errorf("Status = %s, want %s", res.Status, domain.SeriesActive)
	}
	if got := res.RegistrationDate.Format("2006-01-02"); got != "2026-06-04" {
		t.Errorf("RegistrationDate = %s, want 2026-06-04 (from consult dataRegisto)", got)
	}
}

// TestRegisterSeriesReconcileMissesKeepsOriginalError pins the fall-through:
// when the consult finds a DIFFERENT series (or none), the original register
// error must surface, not a reconciled success or the consult's own error.
func TestRegisterSeriesReconcileMissesKeepsOriginalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if strings.Contains(string(b), "consultarSeries") {
			_, _ = w.Write([]byte(statusOKResponse)) // echoes serie S2026 only
			return
		}
		_, _ = w.Write([]byte(atErrorResponse))
	}))
	defer srv.Close()

	_, err := testClient(t, srv.URL).RegisterSeries(context.Background(), SeriesRegistration{
		SeriesID: "OTHER", DocType: domain.FT, SeriesType: "N", InitialSeq: 1, ExpectedStartDate: atT0,
	})
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "4001" {
		t.Fatalf("err = %v, want original at.Error 4001", err)
	}
}

func TestHTTPErrorBecomesATError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := testClient(t, srv.URL).GetSeriesStatus(context.Background(), "S2026", domain.FT)
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "HTTP_500" {
		t.Fatalf("err = %v, want at.Error HTTP_500", err)
	}
}

func TestNewClientValidatesConfig(t *testing.T) {
	for _, cfg := range []Config{
		{Username: "1", Password: "p"},            // missing NIF
		{TaxpayerNIF: "555555550", Password: "p"}, // missing username
		{TaxpayerNIF: "555555550", Username: "1"}, // missing password
		{TaxpayerNIF: "555555550", Username: "1", Password: "p", SeriesURL: "http://x", SoftwareCertNum: "ABC"},   // non-numeric cert
		{TaxpayerNIF: "555555550", Username: "1", Password: "p", SeriesURL: "http://x", SoftwareCertNum: "12345"}, // > 4 digits
	} {
		if _, err := NewClient(cfg); err == nil {
			t.Errorf("NewClient(%+v): want error", cfg)
		}
	}
}

func TestNewClientDefaultsEmptyCertNum(t *testing.T) {
	c, err := NewClient(Config{TaxpayerNIF: "555555550", Username: "1", Password: "p", SeriesURL: "http://x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.config.SoftwareCertNum != "0" {
		t.Errorf("SoftwareCertNum = %q, want 0 (uncertified default)", c.config.SoftwareCertNum)
	}
}

func TestGetSeriesStatusRejectsMismatchedSeries(t *testing.T) {
	var body string
	srv := soapServer(t, statusOKResponse, &body) // response carries serie S2026
	defer srv.Close()

	_, err := testClient(t, srv.URL).GetSeriesStatus(context.Background(), "OTHER", domain.FT)
	atErr, ok := err.(Error)
	if !ok || atErr.Code != "EMPTY_RESPONSE" {
		t.Fatalf("err = %v, want EMPTY_RESPONSE for non-matching infoSerie", err)
	}
}

func TestNewClientDefaultsRateLimiter(t *testing.T) {
	c := testClient(t, "http://x")
	if c.limiter == nil || c.limiter.Limit() != 5 || c.limiter.Burst() != 10 {
		t.Fatalf("limiter defaults: got %v/%v, want 5/10", c.limiter.Limit(), c.limiter.Burst())
	}
	if c.config.TaxEntity != "Global" {
		t.Errorf("TaxEntity = %q, want Global default", c.config.TaxEntity)
	}
	if c.certNum != 9999 {
		t.Errorf("certNum = %d, want 9999", c.certNum)
	}
}

func TestParseATDate_GarbageYieldsZeroTime(t *testing.T) {
	c := &Client{logger: slog.New(slog.DiscardHandler)}
	got := c.parseATDate(context.Background(), "dataRegisto", "2006-01-02", "not-a-date")
	if !got.IsZero() {
		t.Fatalf("fabricated a date for garbage input: %v", got)
	}
	ok := c.parseATDate(context.Background(), "dataRegisto", "2006-01-02", "2026-06-11")
	if ok.IsZero() {
		t.Fatal("valid date rejected")
	}
}
