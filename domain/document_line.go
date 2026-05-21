package domain

import (
	"encoding/json"
	"fmt"
)

type DocumentLine struct {
	ID        int      `json:"id"`
	Product   Product  `json:"product"`
	Quantity  Quantity `json:"quantity"`
	UnitPrice Money    `json:"unit_price"`
	Discount  Discount `json:"discount,omitzero"`
	TaxRate   TaxRate  `json:"tax_rate"`
}

func (l DocumentLine) LineSubtotal() Money {
	return l.UnitPrice.Mul(l.Quantity)
}

// LineTotal = (unit × qty − discount) × (1 + tax). Tax base is post-discount per PT VAT rules.
func (l DocumentLine) LineTotal() Money {
	afterDiscount := l.Discount.Apply(l.LineSubtotal())
	return afterDiscount.Add(afterDiscount.MulPercent(l.TaxRate.Value))
}

func (l DocumentLine) Validate() error {
	if l.UnitPrice < 0 {
		return fmt.Errorf("negative unit price: %s", l.UnitPrice)
	}
	if l.Quantity <= 0 {
		return fmt.Errorf("non-positive quantity: %d", l.Quantity)
	}
	if err := l.Discount.Validate(); err != nil {
		return fmt.Errorf("discount: %w", err)
	}
	if err := l.TaxRate.Validate(); err != nil {
		return fmt.Errorf("tax rate: %w", err)
	}
	return nil
}

func (l *DocumentLine) UnmarshalJSON(data []byte) error {
	type alias DocumentLine
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*l = DocumentLine(tmp)
	return l.Validate()
}
