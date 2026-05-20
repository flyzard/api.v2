package domain

import "github.com/google/uuid"

type Customer struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	TaxID     TaxID     `json:"tax_id"`
	Address   Address   `json:"address"`
}

func NewCustomer(firstName, lastName string, taxID TaxID, address Address) (*Customer, error) {
	if !taxID.IsValid() {
		return nil, ErrInvalidTaxID
	}

	return &Customer{
		ID:        uuid.New(),
		FirstName: firstName,
		LastName:  lastName,
		TaxID:     taxID,
		Address:   address,
	}, nil
}

func (c *Customer) FullName() string {
	return c.FirstName + " " + c.LastName
}
