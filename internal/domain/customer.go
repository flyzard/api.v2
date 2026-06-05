package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// CustomerTaxID holds a tax identifier as a string (XSD: max 30 chars).
type CustomerTaxID string

const (
	MaxLenCustomerTaxID = 30
	MaxLenAccountID     = 30
)

// validateCustomerTaxIDShape trims and enforces non-empty + ≤MaxLenCustomerTaxID.
// Returns the trimmed value so callers don't redo the work.
func validateCustomerTaxIDShape(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrInvalidTaxID
	}
	if len(s) > MaxLenCustomerTaxID {
		return "", fmt.Errorf("customer tax id exceeds %d chars: %q", MaxLenCustomerTaxID, s)
	}
	return s, nil
}

// UnmarshalJSON does shape-only validation (non-empty, ≤MaxLenCustomerTaxID). The
// country-aware NIF checksum check needs the billing-address country and so lives in
// NewCustomer / ValidateCustomerTaxID, where both fields are in scope.
func (c *CustomerTaxID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	clean, err := validateCustomerTaxIDShape(s)
	if err != nil {
		return err
	}
	*c = CustomerTaxID(clean)
	return nil
}

// ValidateCustomerTaxID applies country-aware rules:
func ValidateCustomerTaxID(id CustomerTaxID, country Country) error {
	s, err := validateCustomerTaxIDShape(string(id))
	if err != nil {
		return err
	}
	if country == "PT" && !TaxID(s).IsValid() {
		return ErrInvalidTaxID
	}
	return nil
}

type Customer struct {
	CustomerID           uuid.UUID     `json:"customer_id"`
	AccountID            string        `json:"account_id"`
	CustomerTaxID        CustomerTaxID `json:"customer_tax_id"`
	CompanyName          string        `json:"company_name"`
	SelfBillingIndicator bool          `json:"self_billing_indicator"`
	Contact              string        `json:"contact,omitempty"`
	BillingAddress       Address       `json:"billing_address"`
	ShipToAddresses      []Address     `json:"ship_to_addresses,omitempty"`
	Telephone            string        `json:"telephone,omitempty"`
	Fax                  string        `json:"fax,omitempty"`
	Email                string        `json:"email,omitempty"`
	Website              string        `json:"website,omitempty"`
}

// AnonymousCustomerID is the reserved UUID marker for the "Consumidor final"
var AnonymousCustomerID = uuid.MustParse("00000000-0000-0000-0000-FFFFFFFFFFFF")

// FinalConsumerNIF is the generic "Consumidor final" NIF mandated by
// Portaria 302/2016 field 2.2.3.
const FinalConsumerNIF CustomerTaxID = "999999990"

// NewAnonymousCustomer builds the "Consumidor final" Customer used by
func NewAnonymousCustomer() Customer {
	return Customer{
		CustomerID:    AnonymousCustomerID,
		AccountID:     "ConsumidorFinal",
		CustomerTaxID: FinalConsumerNIF,
		CompanyName:   "Consumidor final",
	}
}

// IsAnonymous reports whether this is the reserved Consumidor-final pseudo-customer.
func (c Customer) IsAnonymous() bool {
	return c.CustomerID == AnonymousCustomerID
}

// Validate is the single gate for both NewCustomer and JSON ingest.
// CustomerID presence is enforced at document level, not here.
func (c Customer) Validate() error {
	if c.AccountID == "" {
		return ErrMissingAccountID
	}
	if len(c.AccountID) > MaxLenAccountID || strings.ContainsRune(c.AccountID, '^') {
		return fmt.Errorf("invalid account id: %q", c.AccountID)
	}
	if c.CompanyName == "" {
		return ErrMissingCompanyName
	}
	// Anonymous ("Consumidor final") skips address + PT-checksum; everything
	// past this branch (structural NIF, AccountID rules) still applies.
	if c.IsAnonymous() {
		if err := ValidateCustomerTaxID(c.CustomerTaxID, ""); err != nil {
			return err
		}
	} else {
		if err := ValidateCustomerTaxID(c.CustomerTaxID, c.BillingAddress.Country); err != nil {
			return err
		}
		if err := c.BillingAddress.Validate(); err != nil {
			return fmt.Errorf("billing address: %w", err)
		}
	}
	return nil
}

// UnmarshalJSON runs Validate so the PT NIF checksum fires on JSON ingest too.
func (c *Customer) UnmarshalJSON(data []byte) error {
	type alias Customer
	if err := json.Unmarshal(data, (*alias)(c)); err != nil {
		return err
	}
	return c.Validate()
}

func NewCustomer(accountID string, taxID CustomerTaxID, companyName string, billing Address, selfBilling bool) (*Customer, error) {
	c := Customer{
		CustomerID:           uuid.New(),
		AccountID:            accountID,
		CustomerTaxID:        taxID,
		CompanyName:          companyName,
		BillingAddress:       billing,
		SelfBillingIndicator: selfBilling,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
