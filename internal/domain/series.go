package domain

import (
	"fmt"
	"regexp"
	"time"
)

type SeriesATStatus string

const (
	SeriesPending   SeriesATStatus = "pending"
	SeriesActive    SeriesATStatus = "active"
	SeriesFinalized SeriesATStatus = "finalized"
	SeriesCancelled SeriesATStatus = "cancelled"
)

type ProcessingMeans string

const (
	ProcessingNormal   ProcessingMeans = "N"
	ProcessingRecovery ProcessingMeans = "A"
)

type Series struct {
	ID               string         `json:"id"`
	DocType          DocumentType   `json:"doc_type"`
	ATCode           string         `json:"at_code,omitempty"`
	ATStatus         SeriesATStatus `json:"at_status"`
	RegistrationDate *time.Time     `json:"registration_date,omitempty"`
	LastNum          int            `json:"last_num"`
	LastHash         string         `json:"last_hash,omitempty"`
	LastDate         *time.Time     `json:"last_date,omitempty"`
	LastSystemDate   *time.Time     `json:"last_system_date,omitempty"`
	// Version is bumped on every successful AppendIssue. Persistence layers compare it
	// in the UPDATE WHERE Version=? clause to detect concurrent issuance against the
	// same series. The domain stays single-threaded; this is bookkeeping only.
	Version         uint64          `json:"version"`
	FinalizedAt     *time.Time      `json:"finalized_at,omitempty"`
	CancelledAt     *time.Time      `json:"cancelled_at,omitempty"`
	ProcessingMeans ProcessingMeans `json:"processing_means"`
}

// seriesCharset excludes space, "/" and "^" (they would break the SAF-T
// DocumentNumber pattern — docs/series-rules.yaml series-identifier-charset)
// and sticks to the safe subset AT communication accepts. No reserved-prefix
// check: the claimed "AT" prefix rejection is unconfirmed (see
// series-identifier-no-reserved-prefix); registarSerie is the authority.
var seriesCharset = regexp.MustCompile(`^[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)*$`)

func ValidateSeries(id string) error {
	if n := len(id); n < 1 || n > 20 {
		return fmt.Errorf("series id length must be 1-20: %q", id)
	}
	if !seriesCharset.MatchString(id) {
		return fmt.Errorf("series id is invalid (allowed: alphanumerics separated by single . _ -): %q", id)
	}
	return nil
}

func NewSeries(id string, docType DocumentType) (Series, error) {
	if err := ValidateSeries(id); err != nil {
		return Series{}, err
	}
	if !docType.IsValid() {
		return Series{}, fmt.Errorf("invalid document type: %q", docType)
	}
	return Series{
		ID:              id,
		DocType:         docType,
		ATStatus:        SeriesPending,
		ProcessingMeans: ProcessingNormal,
	}, nil
}

// NewRecoverySeries creates a series dedicated to integrating recovered
// documents (Portaria 363/2010). Recovery issuance is only legal into such a
// series; normal issuance into it is rejected (see validateIssueContext).
func NewRecoverySeries(id string, docType DocumentType) (Series, error) {
	s, err := NewSeries(id, docType)
	if err != nil {
		return Series{}, err
	}
	s.ProcessingMeans = ProcessingRecovery
	return s, nil
}

func (s Series) IsRegistered() bool { return s.ATCode != "" }
func (s Series) CanIssue() bool     { return s.ATStatus == SeriesActive && s.IsRegistered() }

func (s *Series) RegisterWithAT(atCode string, at time.Time) error {
	if s.ATCode != "" {
		return ErrSeriesAlreadyRegistered
	}
	if err := ValidateATCode(atCode); err != nil {
		return err
	}
	s.ATCode = atCode
	s.RegistrationDate = &at
	s.ATStatus = SeriesActive
	return nil
}

// Finalize closes the series locally after AT has accepted finalizarSerie
// (Portaria 195/2020). A finalized series can never issue again.
func (s *Series) Finalize(at time.Time) error {
	if s.ATStatus != SeriesActive {
		return fmt.Errorf("%w: series %q is %s", ErrSeriesNotActive, s.ID, s.ATStatus)
	}
	s.ATStatus = SeriesFinalized
	s.FinalizedAt = &at
	return nil
}

// Cancel voids the AT registration locally after AT has accepted anularSerie.
// AT only accepts the cancellation of a series that never issued a document
// (declaracaoNaoEmissao); a used series must be finalized instead.
func (s *Series) Cancel(at time.Time) error {
	if s.ATStatus != SeriesActive {
		return fmt.Errorf("%w: series %q is %s", ErrSeriesNotActive, s.ID, s.ATStatus)
	}
	if s.LastNum > 0 {
		return fmt.Errorf("%w: series %q issued %d documents", ErrSeriesHasDocuments, s.ID, s.LastNum)
	}
	s.ATStatus = SeriesCancelled
	s.CancelledAt = &at
	return nil
}

// AppendIssue advances the series after a successful issuance. Caller MUST pass
// seq == LastNum+1 inside the transaction that persists the document.
// Empty hash leaves LastHash untouched — payments consume seq but don't chain.
// docDate is the invoice/transaction date used for monotonicity checks on the next issue.
func (s *Series) AppendIssue(seq int, hash string, docDate, now time.Time) {
	s.LastNum = seq
	if hash != "" {
		s.LastHash = hash
	}
	s.LastDate = &docDate
	s.LastSystemDate = &now
	s.Version++
}
