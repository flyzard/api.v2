package domain

import (
	"encoding/json"
	"fmt"
)

type DiscountKind string

const (
	DiscountNone    DiscountKind = ""
	DiscountPercent DiscountKind = "percent"
	DiscountAmount  DiscountKind = "amount"
)

type Discount struct {
	Kind    DiscountKind `json:"kind"`
	Percent Percent      `json:"percent"`
	Amount  Money        `json:"amount"`
}

// NewPercentDiscount takes a human percent (10.0 = 10% off).
func NewPercentDiscount(value float64) (Discount, error) {
	p, err := NewPercent(value)
	if err != nil {
		return Discount{}, err
	}
	return Discount{Kind: DiscountPercent, Percent: p}, nil
}

func NewAmountDiscount(m Money) (Discount, error) {
	d := Discount{Kind: DiscountAmount, Amount: m}
	return d, d.Validate()
}

// Validate checks invariants. Run after deserializing from untrusted sources.
func (d Discount) Validate() error {
	switch d.Kind {
	case DiscountNone:
	case DiscountPercent:
		if d.Percent < 0 || d.Percent > PercentScale {
			return fmt.Errorf("percent out of range: %d", d.Percent)
		}
	case DiscountAmount:
		if d.Amount < 0 {
			return fmt.Errorf("negative discount amount: %d", d.Amount)
		}
	default:
		return fmt.Errorf("invalid discount kind: %s", d.Kind)
	}
	return nil
}

// UnmarshalJSON runs Validate after standard unmarshal.
func (d *Discount) UnmarshalJSON(data []byte) error {
	type alias Discount
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*d = Discount(tmp)
	return d.Validate()
}

// Apply returns the net amount after discount. Panics on unrecognized Kind —
// JSON and constructors validate up-front, so an unknown Kind here means corrupted state.
func (d Discount) Apply(base Money) Money {
	switch d.Kind {
	case DiscountNone:
		return base
	case DiscountPercent:
		return base.Sub(base.MulPercent(d.Percent))
	case DiscountAmount:
		if d.Amount > base {
			return 0
		}
		return base.Sub(d.Amount)
	default:
		panic(fmt.Sprintf("invalid discount kind: %q", d.Kind))
	}
}
