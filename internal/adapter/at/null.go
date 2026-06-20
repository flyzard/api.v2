package at

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// NullClient is an in-memory SeriesWS fake for dev/demo/tests.
type NullClient struct {
	mu     sync.Mutex
	series map[nullKey]*nullSeries
	Now    func() time.Time
}

type nullKey struct {
	DocType domain.DocumentType
	ID      string
}

type nullSeries struct {
	code    string
	status  domain.SeriesATStatus
	lastSeq int
}

// NewNullClient creates an empty in-memory fake.
func NewNullClient() *NullClient {
	return &NullClient{series: map[nullKey]*nullSeries{}}
}

func (c *NullClient) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Vowels and digits 0/1 excluded to avoid accidental words and visual
// confusion with O/I. Real AT codes may use any uppercase alphanumeric per
// Portaria 195/2020; this narrower pool keeps placeholder codes safe for
// certification dossiers.
const nullCodePool = "BCDFGHJKLMNPQRSTVWXYZ23456789"

func deriveCodeFromKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	var b [8]byte
	for i := range b {
		b[i] = nullCodePool[int(sum[i])%len(nullCodePool)]
	}
	return string(b[:])
}

// deriveUniqueCode salts the key and rederives on collision with an
// already-allocated code, so two series never share a placeholder code
// (their ATCUDs would collide). Caller must hold c.mu.
func (c *NullClient) deriveUniqueCode(key nullKey) string {
	seen := make(map[string]bool, len(c.series))
	for _, s := range c.series {
		seen[s.code] = true
	}
	base := string(key.DocType) + "_" + key.ID
	for i := range 32 {
		salt := base
		if i > 0 {
			salt = fmt.Sprintf("%s#%d", base, i)
		}
		if code := deriveCodeFromKey(salt); !seen[code] {
			return code
		}
	}
	return deriveCodeFromKey(base)
}

// RegisterSeries simulates registarSerie. Repeat calls with the same (DocType, SeriesID) return the same code.
func (c *NullClient) RegisterSeries(_ context.Context, req SeriesRegistration) (*SeriesRegistrationResult, error) {
	if !req.DocType.IsValid() {
		return nil, fmt.Errorf("null AT client: unsupported doc type %q", req.DocType)
	}
	// Delegate SeriesID format to the domain validator so the fake never
	// rejects a series the domain would accept.
	if err := domain.ValidateSeries(req.SeriesID); err != nil {
		return nil, fmt.Errorf("null AT client: invalid series identifier %q: %w", req.SeriesID, err)
	}

	key := nullKey{DocType: req.DocType, ID: req.SeriesID}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.series[key]; ok {
		return &SeriesRegistrationResult{
			ValidationCode:   existing.code,
			RegistrationDate: c.now(),
			Status:           existing.status,
		}, nil
	}

	code := c.deriveUniqueCode(key)
	if err := domain.ValidateATCode(code); err != nil {
		return nil, fmt.Errorf("null AT client: generated invalid code %q: %w", code, err)
	}
	c.series[key] = &nullSeries{code: code, status: domain.SeriesActive}

	return &SeriesRegistrationResult{
		ValidationCode:   code,
		RegistrationDate: c.now(),
		Status:           domain.SeriesActive,
	}, nil
}

func (c *NullClient) lookup(seriesID string, docType domain.DocumentType) (*nullSeries, error) {
	s, ok := c.series[nullKey{DocType: docType, ID: seriesID}]
	if !ok {
		return nil, Error{Code: "EMPTY_RESPONSE", Message: fmt.Sprintf("series %q (%s) not registered", seriesID, docType)}
	}
	return s, nil
}

// FinalizeSeries simulates finalizarSerie.
func (c *NullClient) FinalizeSeries(_ context.Context, req SeriesFinalization) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.lookup(req.SeriesID, req.DocType)
	if err != nil {
		return err
	}
	s.status = domain.SeriesFinalized
	s.lastSeq = req.LastSeq
	return nil
}

// CancelSeries simulates anularSerie.
func (c *NullClient) CancelSeries(_ context.Context, req SeriesCancellation) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.lookup(req.SeriesID, req.DocType)
	if err != nil {
		return err
	}
	s.status = domain.SeriesCancelled
	return nil
}

// GetSeriesStatus simulates consultarSeries.
func (c *NullClient) GetSeriesStatus(_ context.Context, seriesID string, docType domain.DocumentType) (*SeriesStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.lookup(seriesID, docType)
	if err != nil {
		return nil, err
	}
	return &SeriesStatus{
		SeriesID:       seriesID,
		DocType:        docType,
		ValidationCode: s.code,
		Status:         s.status,
		LastSeq:        s.lastSeq,
	}, nil
}

// CommunicateTransport simulates sgdtws: deterministic fake ATDocCodeID derived from the document number.
func (c *NullClient) CommunicateTransport(_ context.Context, _ domain.Company, mv domain.StockMovement) (*TransportResult, error) {
	if !mv.DocumentType.IsTransport() {
		return nil, fmt.Errorf("null AT client: %s is not a transport document", mv.Number.Format())
	}
	// Mirror sgdtws: a cancellation voids an already-communicated document and gets no new ATDocCodeID (see Client.CommunicateTransport).
	if mv.Status == domain.StatusCancelled {
		return &TransportResult{
			DocumentNumber: mv.Number.Format(),
			ATCUD:          string(mv.ATCUD),
			Message:        "OK",
		}, nil
	}
	return &TransportResult{
		ATDocCodeID:    "ATDC" + deriveCodeFromKey("transport_"+mv.Number.Format()),
		DocumentNumber: mv.Number.Format(),
		ATCUD:          string(mv.ATCUD),
		Message:        "OK",
	}, nil
}

// CommunicateInvoice simulates fatcorews: always succeeds.
func (c *NullClient) CommunicateInvoice(_ context.Context, _ domain.Company, inv domain.SalesInvoice) (*InvoiceResult, error) {
	if !inv.DocumentType.IsSales() {
		return nil, fmt.Errorf("null AT client: %s is not a sales document", inv.Number.Format())
	}
	return &InvoiceResult{Message: "Documento registado com sucesso.", OperationDate: c.now()}, nil
}

// Compile-time interface checks.
var (
	_ SeriesClient    = (*NullClient)(nil)
	_ TransportClient = (*Client)(nil)
	_ TransportClient = (*NullClient)(nil)
	_ InvoiceClient   = (*Client)(nil)
	_ InvoiceClient   = (*NullClient)(nil)
)
