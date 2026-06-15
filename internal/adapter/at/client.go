package at

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/time/rate"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Well-known SeriesWS endpoints per AT "Comunicação de Séries Documentais —
// Aspetos Específicos" v1.2 (Dec 2022) §1.2.2.
const (
	TestSeriesURL       = "https://servicos.portaldasfinancas.gov.pt:722/SeriesWSService"
	ProductionSeriesURL = "https://servicos.portaldasfinancas.gov.pt:422/SeriesWSService"
)

// Config holds the SeriesWS connection settings.
type Config struct {
	// SeriesURL is the full webservice URL: TestSeriesURL, ProductionSeriesURL,
	// or an httptest server URL in tests.
	SeriesURL string

	// TransportURL / InvoiceURL are the full webservice URLs for transport
	// docs (sgdtws) and invoice communication (fatcorews). Optional — only
	// required by the respective operation.
	TransportURL string
	InvoiceURL   string

	// Authentication (sub-user with the WSE operation permission).
	TaxpayerNIF string
	Username    string
	Password    string

	// SoftwareCertNum is the AT-issued certificate number stamped into
	// registarSerie (numCertSWFatur). "0" if uncertified.
	SoftwareCertNum string

	// TaxEntity is the fatcorews establishment identifier: "Global" (default),
	// "Sede", or an establishment code.
	TaxEntity string

	// ATPublicKey enables WS-Security credential encryption (required in
	// production). When nil, the password travels in plain text — tests only.
	ATPublicKey *rsa.PublicKey

	// Certificate is the client certificate for mutual TLS (optional).
	Certificate tls.Certificate

	// Timeout is the per-HTTP-request timeout (default 30s).
	Timeout time.Duration
	// OperationTimeout caps a whole operation including retries. A caller
	// context deadline takes precedence. Default is derived from Timeout and
	// Retry: MaxRetries×Timeout + 2×MaxBackoff, so every retry can survive a
	// full slow failure without being killed by the op deadline.
	OperationTimeout time.Duration

	// Retry tunes the transient-error backoff. Zero values use defaults
	// (3 attempts, 500ms initial, 10s max).
	Retry RetrySettings

	// RateLimit / RateLimitBurst throttle outbound calls to stay inside AT's
	// per-NIF limits (~5-10 req/s). Defaults: 5 req/s, burst 10.
	RateLimit      float64
	RateLimitBurst int

	Logger    *slog.Logger // nil → slog.Default()
	LogBodies bool         // log SOAP request/response XML (passwords masked)
}

// Client talks to the AT SeriesWS over SOAP. Implements SeriesClient.
type Client struct {
	config     Config
	httpClient *http.Client
	logger     *slog.Logger
	limiter    *rate.Limiter
	certNum    int           // parsed Config.SoftwareCertNum
	opTimeout  time.Duration // whole-operation deadline, covers all retries
}

// passwordMaskRe masks WS-Security passwords in SOAP XML for logging.
var passwordMaskRe = regexp.MustCompile(`(<wsse:Password>)[^<]*(</wsse:Password>)`)

// NewClient validates the config and builds a SeriesWS client.
func NewClient(config Config) (*Client, error) {
	if config.TaxpayerNIF == "" {
		return nil, fmt.Errorf("at.Config: TaxpayerNIF required")
	}
	if config.Username == "" {
		return nil, fmt.Errorf("at.Config: Username required")
	}
	if config.Password == "" {
		return nil, fmt.Errorf("at.Config: Password required")
	}
	if config.SeriesURL == "" {
		return nil, fmt.Errorf("at.Config: SeriesURL required (TestSeriesURL or ProductionSeriesURL)")
	}
	// numCertSWFatur is xsd:integer with totalDigits=4 — an empty or
	// non-numeric value would only fail at AT's schema validator.
	if config.SoftwareCertNum == "" {
		config.SoftwareCertNum = "0"
	}
	certNum, err := strconv.Atoi(config.SoftwareCertNum)
	if err != nil || certNum < 0 || certNum > 9999 {
		return nil, fmt.Errorf("at.Config: SoftwareCertNum must be 0-9999, got %q", config.SoftwareCertNum)
	}
	if config.TaxEntity == "" {
		config.TaxEntity = "Global"
	}
	rateLimit := config.RateLimit
	if rateLimit <= 0 {
		rateLimit = 5
	}
	burst := config.RateLimitBurst
	if burst <= 0 {
		burst = 10
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	opTimeout := config.OperationTimeout
	if opTimeout <= 0 {
		rs := config.Retry.withDefaults()
		// Every attempt may eat a full request timeout; add the worst-case
		// backoff schedule so the last retry is not killed by the op deadline.
		opTimeout = time.Duration(rs.MaxRetries)*timeout + 2*rs.MaxBackoff
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if len(config.Certificate.Certificate) > 0 {
		tr.TLSClientConfig.Certificates = []tls.Certificate{config.Certificate}
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   timeout,
		},
		logger:    logger,
		limiter:   rate.NewLimiter(rate.Limit(rateLimit), burst),
		certNum:   certNum,
		opTimeout: opTimeout,
	}, nil
}

// ensureDeadline applies opTimeout if the caller's context has no deadline.
func (c *Client) ensureDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.opTimeout)
}

// prepareCredentials builds soapCredentials, encrypting if ATPublicKey is set.
// Called inside each *Once method so each retry gets a fresh AES key + timestamp.
func (c *Client) prepareCredentials() (soapCredentials, error) {
	creds := soapCredentials{
		NIF:             c.config.TaxpayerNIF,
		Username:        c.config.Username,
		TaxEntity:       c.config.TaxEntity,
		SoftwareCertNum: c.certNum,
	}
	if c.config.ATPublicKey != nil {
		encPass, nonce, encCreated, err := EncryptATCredentials(
			c.config.Password, time.Now(), c.config.ATPublicKey,
		)
		if err != nil {
			return soapCredentials{}, fmt.Errorf("encrypting AT credentials: %w", err)
		}
		creds.Password = encPass
		creds.Nonce = nonce
		creds.Created = encCreated
	} else {
		creds.Password = c.config.Password
	}
	return creds, nil
}

// sendSOAPRequest posts a SOAP envelope to the given URL and returns the body.
func (c *Client) sendSOAPRequest(ctx context.Context, operation, url string, envelope []byte) ([]byte, error) {
	// Proactive throttle to stay within AT per-NIF limits.
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("AT rate limiter: %w", err)
	}
	start := time.Now()

	if c.config.LogBodies {
		masked := passwordMaskRe.ReplaceAllString(string(envelope), "${1}***${2}")
		c.logger.DebugContext(ctx, "AT SOAP request",
			slog.String("operation", operation), slog.String("body", masked))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(envelope))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.ErrorContext(ctx, "AT SOAP connection failed",
			slog.String("operation", operation),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()))
		return nil, Error{Code: "CONNECTION", Message: fmt.Sprintf("connecting to AT: %v", err)}
	}
	defer resp.Body.Close() //nolint:errcheck // close error non-actionable

	const maxResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.ErrorContext(ctx, "AT SOAP HTTP error",
			slog.String("operation", operation),
			slog.Duration("duration", time.Since(start)),
			slog.Int("status", resp.StatusCode))
		snippet := body
		if len(snippet) > 2048 {
			snippet = snippet[:2048]
		}
		return nil, Error{
			Code:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			Message: fmt.Sprintf("AT returned status %d: %s", resp.StatusCode, snippet),
		}
	}

	if c.config.LogBodies {
		c.logger.DebugContext(ctx, "AT SOAP response",
			slog.String("operation", operation), slog.String("body", string(body)))
	}
	c.logger.InfoContext(ctx, "AT SOAP request completed",
		slog.String("operation", operation),
		slog.Duration("duration", time.Since(start)),
		slog.Int("status", resp.StatusCode))

	return body, nil
}

// atError logs and wraps an AT-reported business error. Shared by every
// webservice; SeriesWS goes through operResult, sgdtws/fatcorews call it
// directly with their own status fields.
func (c *Client) atError(ctx context.Context, operation string, code int, message string) Error {
	atErr := Error{Code: strconv.Itoa(code), Message: message}
	c.logger.WarnContext(ctx, "AT returned error",
		slog.String("operation", operation),
		slog.String("code", atErr.Code),
		slog.String("message", atErr.Message))
	return atErr
}

// operResult converts an AT 3xxx+ operation code into an Error, logging it.
func (c *Client) operResult(ctx context.Context, operation string, r atOperationResult) error {
	if !r.IsError() {
		return nil
	}
	return c.atError(ctx, operation, r.CodResultOper, r.MsgResultOper)
}

// parseATDate parses an AT response timestamp. An unparseable value returns
// the zero time — recognizably absent — never a fabricated local clock value
// that could be persisted as a legal registration date.
func (c *Client) parseATDate(ctx context.Context, field, layout, raw string) time.Time {
	t, err := time.Parse(layout, raw)
	if err != nil {
		c.logger.WarnContext(ctx, "AT date unparseable; returning zero time",
			slog.String(field, raw))
		return time.Time{}
	}
	return t
}

// soapCall runs the per-operation skeleton every webservice method shares:
// deadline, retry loop, fresh WS-Security credentials per attempt (replay
// protection), envelope build, HTTP round-trip, SOAP fault/parse — then the
// operation-specific inspect step on the decoded response. A free function
// because Go methods cannot take type parameters.
func soapCall[Resp, Out any](
	c *Client, ctx context.Context, operation, url string,
	build func(soapCredentials) ([]byte, error),
	inspect func(ctx context.Context, resp *Resp) (Out, error),
) (Out, error) {
	ctx, cancel := c.ensureDeadline(ctx)
	defer cancel()
	return retryable(ctx, c.logger, c.config.Retry, operation, func() (Out, error) {
		var zero Out
		creds, err := c.prepareCredentials()
		if err != nil {
			return zero, err
		}
		envelope, err := build(creds)
		if err != nil {
			return zero, fmt.Errorf("building SOAP envelope: %w", err)
		}
		respBody, err := c.sendSOAPRequest(ctx, operation, url, envelope)
		if err != nil {
			return zero, err
		}
		var resp Resp
		if err := parseSOAPResponse(respBody, &resp); err != nil {
			return zero, err
		}
		return inspect(ctx, &resp)
	})
}

// RegisterSeries registers a document series with AT (registarSerie).
//
// Retry hazard + reconciliation: registarSerie is not idempotent. A retryable
// failure ("CONNECTION", HTTP 502/504) can mean the request reached AT and was
// committed but the response was lost — the retry then gets a deterministic
// "série já registada" (4xxx) error even though registration succeeded. On any
// failure this method therefore consults the series (consultarSeries) and, if
// AT shows it registered, returns its state as success — mirroring
// NullClient's idempotent RegisterSeries. Genuine failures fall through: the
// consult finds nothing and the original register error surfaces.
func (c *Client) RegisterSeries(ctx context.Context, req SeriesRegistration) (*SeriesRegistrationResult, error) {
	res, err := c.registerSeries(ctx, req)
	if err == nil {
		return res, nil
	}
	var atErr Error
	if !errors.As(err, &atErr) {
		return nil, err
	}
	st, stErr := c.GetSeriesStatus(ctx, req.SeriesID, req.DocType)
	if stErr != nil || st.Status != domain.SeriesActive || st.ValidationCode == "" {
		return nil, err // reconcile miss: keep the register error
	}
	c.logger.InfoContext(ctx, "RegisterSeries reconciled via consultarSeries",
		slog.String("series", req.SeriesID),
		slog.String("registerError", err.Error()))
	return &SeriesRegistrationResult{
		ValidationCode:   st.ValidationCode,
		RegistrationDate: st.RegistrationDate,
		Status:           st.Status,
	}, nil
}

func (c *Client) registerSeries(ctx context.Context, req SeriesRegistration) (*SeriesRegistrationResult, error) {
	return soapCall(c, ctx, "RegisterSeries", c.config.SeriesURL,
		func(creds soapCredentials) ([]byte, error) {
			return buildSeriesRegistrationEnvelope(creds, req, c.config.SoftwareCertNum)
		},
		func(ctx context.Context, resp *seriesRegistrationResponse) (*SeriesRegistrationResult, error) {
			if err := c.operResult(ctx, "RegisterSeries", resp.Resp.InfoResultOper); err != nil {
				return nil, err
			}
			info := resp.Resp.InfoSerie
			if info == nil {
				return nil, Error{Code: "EMPTY_RESPONSE", Message: "AT returned no infoSerie for registarSerie"}
			}
			return &SeriesRegistrationResult{
				ValidationCode:   info.CodValidacaoSerie,
				RegistrationDate: c.parseATDate(ctx, "dataRegisto", "2006-01-02", info.DataRegisto),
				Status:           statusFromEstado(info.Estado),
			}, nil
		})
}

// FinalizeSeries closes a series with AT (finalizarSerie).
func (c *Client) FinalizeSeries(ctx context.Context, req SeriesFinalization) error {
	_, err := soapCall(c, ctx, "FinalizeSeries", c.config.SeriesURL,
		func(creds soapCredentials) ([]byte, error) {
			return buildSeriesFinalizationEnvelope(creds, req)
		},
		func(ctx context.Context, resp *seriesFinalizationResponse) (struct{}, error) {
			return struct{}{}, c.operResult(ctx, "FinalizeSeries", resp.Resp.InfoResultOper)
		})
	return err
}

// CancelSeries voids a series registration with AT (anularSerie). Only legal
// for series that never issued a document.
func (c *Client) CancelSeries(ctx context.Context, req SeriesCancellation) error {
	_, err := soapCall(c, ctx, "CancelSeries", c.config.SeriesURL,
		func(creds soapCredentials) ([]byte, error) {
			return buildSeriesCancellationEnvelope(creds, req)
		},
		func(ctx context.Context, resp *seriesCancellationResponse) (struct{}, error) {
			return struct{}{}, c.operResult(ctx, "CancelSeries", resp.Resp.InfoResultOper)
		})
	return err
}

// GetSeriesStatus queries series state from AT (consultarSeries).
func (c *Client) GetSeriesStatus(ctx context.Context, seriesID string, docType domain.DocumentType) (*SeriesStatus, error) {
	return soapCall(c, ctx, "GetSeriesStatus", c.config.SeriesURL,
		func(creds soapCredentials) ([]byte, error) {
			return buildSeriesStatusEnvelope(creds, seriesID, docType)
		},
		func(ctx context.Context, resp *seriesStatusResponse) (*SeriesStatus, error) {
			if err := c.operResult(ctx, "GetSeriesStatus", resp.Resp.InfoResultOper); err != nil {
				return nil, err
			}
			// consultarSeries can echo multiple infoSerie elements; pick the one
			// matching the queried identifier rather than trusting position.
			for _, info := range resp.Resp.InfoSerie {
				if info.Serie != seriesID {
					continue
				}
				return &SeriesStatus{
					SeriesID:         info.Serie,
					DocType:          domain.DocumentType(info.TipoDoc),
					ValidationCode:   info.CodValidacaoSerie,
					Status:           statusFromEstado(info.Estado),
					LastSeq:          info.SeqUltimoDocEmitido,
					RegistrationDate: c.parseATDate(ctx, "dataRegisto", "2006-01-02", info.DataRegisto),
				}, nil
			}
			return nil, Error{Code: "EMPTY_RESPONSE", Message: fmt.Sprintf("AT returned no infoSerie matching %q for consultarSeries", seriesID)}
		})
}

// Compile-time interface check.
var _ SeriesClient = (*Client)(nil)
