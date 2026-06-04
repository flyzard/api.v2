package domain

import (
	"encoding/json"
	"fmt"
)

// Period is the SAF-T Period element on source documents: integer 1..12 (calendar month).
// The 1..16 SAFPTAccountingPeriod range is used only in GeneralLedgerEntries (out of scope).
type Period int

func NewPeriod(month int) (Period, error) {
	p := Period(month)
	if err := p.Validate(); err != nil {
		return 0, err
	}
	return p, nil
}

func (p Period) Validate() error {
	if p < 1 || p > 12 {
		return fmt.Errorf("period out of range (1..12): %d", p)
	}
	return nil
}

func (p *Period) UnmarshalJSON(data []byte) error {
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	period, err := NewPeriod(v)
	if err != nil {
		return err
	}
	*p = period
	return nil
}
