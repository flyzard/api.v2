package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type SeriesATStatus string

const (
	SeriesPending   SeriesATStatus = "pending"
	SeriesActive    SeriesATStatus = "active"
	SeriesFinalized SeriesATStatus = "finalized"
)

type ProcessingMeans string

const (
	ProcessingNormal   ProcessingMeans = "N"
	ProcessingRecovery ProcessingMeans = "A"
	ProcessingTraining ProcessingMeans = "T"
)

type Series struct {
	ID               string          `json:"id"`
	DocType          DocumentType    `json:"doc_type"`
	ATCode           string          `json:"at_code,omitempty"`
	ATStatus         SeriesATStatus  `json:"at_status"`
	RegistrationDate *time.Time      `json:"registration_date,omitempty"`
	Year             *int            `json:"year,omitempty"`
	LastNum          int             `json:"last_num"`
	LastHash         string          `json:"last_hash,omitempty"`
	LastSystemDate   *time.Time      `json:"last_system_date,omitempty"`
	Active           bool            `json:"active"`
	FinalizedAt      *time.Time      `json:"finalized_at,omitempty"`
	ProcessingMeans  ProcessingMeans `json:"processing_means"`
}

var seriesCharset = regexp.MustCompile(`^[A-Za-z0-9._\-/]+$`)

func ValidateSeries(id string) error {
	if n := len(id); n < 1 || n > 20 {
		return fmt.Errorf("series id length must be 1-20: %q", id)
	}
	if !seriesCharset.MatchString(id) {
		return fmt.Errorf("series id has invalid characters: %q", id)
	}
	if len(id) >= 2 && strings.EqualFold(id[:2], "AT") {
		return fmt.Errorf("series id cannot start with AT: %q", id)
	}
	if isSeriesSep(id[0]) || isSeriesSep(id[len(id)-1]) {
		return fmt.Errorf("series id cannot start or end with separator: %q", id)
	}
	for i := 1; i < len(id); i++ {
		if isSeriesSep(id[i]) && isSeriesSep(id[i-1]) {
			return fmt.Errorf("series id has consecutive separators: %q", id)
		}
	}
	return nil
}

func isSeriesSep(c byte) bool {
	return c == '.' || c == '_' || c == '-' || c == '/'
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

func (s Series) IsRegistered() bool { return s.ATCode != "" }
func (s Series) CanIssue() bool     { return s.Active && s.IsRegistered() }

func (s *Series) RegisterWithAT(atCode string, at time.Time) error {
	if s.ATCode != "" {
		return ErrSeriesAlreadyRegistered
	}
	s.ATCode = atCode
	s.RegistrationDate = &at
	s.ATStatus = SeriesActive
	s.Active = true
	return nil
}
