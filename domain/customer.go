package domain

import "github.com/google/uuid"

type Customer struct {
	CustomerID           uuid.UUID `json:"customer_id"`
	AccountID            string    `json:"account_id"`
	CustomerTaxID        TaxID     `json:"customer_tax_id"`
	CompanyName          string    `json:"company_name"`
	SelfBillingIndicator bool      `json:"self_billing_indicator"`
	Contact              string    `json:"contact,omitempty"`
	BillingAddress       *Address  `json:"billing_address"`
	ShipToAddress        *Address  `json:"ship_to_address"`
	Telephone            string    `json:"telephone,omitempty"`
	Fax                  string    `json:"fax,omitempty"`
	Email                string    `json:"email,omitempty"`
	Website              string    `json:"website,omitempty"`
}

func NewCustomer(accountID string, taxID TaxID, companyName string, selfBilling bool) (*Customer, error) {
	if accountID == "" {
		return nil, ErrMissingAccountID
	}
	if !taxID.IsValid() {
		return nil, ErrInvalidTaxID
	}
	if companyName == "" {
		return nil, ErrMissingCompanyName
	}
	return &Customer{
		CustomerID:           uuid.New(),
		AccountID:            accountID,
		CustomerTaxID:        taxID,
		CompanyName:          companyName,
		SelfBillingIndicator: selfBilling,
	}, nil
}
