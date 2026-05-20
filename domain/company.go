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

type CompanyOption func(*Company)

func WithTradeName(s string) CompanyOption { return func(c *Company) { c.TradeName = s } }
func WithAddress(a Address) CompanyOption  { return func(c *Company) { c.Address = a } }
func WithPhone(s string) CompanyOption     { return func(c *Company) { c.Phone = s } }
func WithFax(s string) CompanyOption       { return func(c *Company) { c.Fax = s } }
func WithEmail(s string) CompanyOption     { return func(c *Company) { c.Email = s } }
func WithWebsite(s string) CompanyOption   { return func(c *Company) { c.Website = s } }
func WithFiscalYear(y int) CompanyOption   { return func(c *Company) { c.FiscalYear = y } }
func WithStartMonth(m int) CompanyOption   { return func(c *Company) { c.StartMonth = m } }
func WithEACCode(s string) CompanyOption   { return func(c *Company) { c.EACCode = s } }
func WithActive(active bool) CompanyOption { return func(c *Company) { c.Active = active } }

func NewCompany(nif TaxID, name string, opts ...CompanyOption) (Company, error) {
	c := Company{
		ID:         uuid.New(),
		NIF:        nif,
		Name:       strings.TrimSpace(name),
		StartMonth: 1,
		Active:     true,
	}
	for _, opt := range opts {
		opt(&c)
	}
	if err := c.Validate(); err != nil {
		return Company{}, err
	}
	return c, nil
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
