package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Company struct {
	ID         uuid.UUID `json:"id"`
	NIF        TaxID     `json:"tax_id"`
	Name       string    `json:"name"`
	TradeName  string    `json:"trade_name,omitempty"`
	Address    Address   `json:"address"`
	Phone      string    `json:"phone,omitempty"`
	Fax        string    `json:"fax,omitempty"`
	Email      string    `json:"email,omitempty"`
	Website    string    `json:"website,omitempty"`
	FiscalYear int       `json:"fiscal_year,omitempty"`
	StartMonth int       `json:"start_month"`
	EACCode    string    `json:"eac_code,omitempty"`
	Active     bool      `json:"active"`
}

func NewCompany(c Company) (Company, error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.StartMonth == 0 {
		c.StartMonth = 1
	}
	return c, c.Validate()
}

func (c Company) Validate() error {
	if !c.NIF.IsValid() {
		return ErrInvalidTaxID
	}
	if c.Name == "" {
		return errors.New("company name is required")
	}
	if c.StartMonth < 1 || c.StartMonth > 12 {
		return fmt.Errorf("start month out of range: %d", c.StartMonth)
	}
	if c.FiscalYear != 0 && (c.FiscalYear < 1900 || c.FiscalYear > 9999) {
		return fmt.Errorf("invalid fiscal year: %d", c.FiscalYear)
	}
	if c.EACCode != "" && len(c.EACCode) != 5 {
		return fmt.Errorf("EAC code must be 5 digits: %q", c.EACCode)
	}
	return nil
}
