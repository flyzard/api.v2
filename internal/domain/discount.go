package domain

import (
	"encoding/json"
	"fmt"
)

type Discount interface {
	Apply(base Money) Money
	Validate() error
	isDiscount()
}

// PercentDiscount removes Rate percent of the base.
type PercentDiscount struct {
	Rate Percent `json:"percent"`
}

func (PercentDiscount) isDiscount() {}

func (p PercentDiscount) Validate() error {
	if p.Rate < 0 || p.Rate > PercentScale {
		return fmt.Errorf("discount percent out of range [0%%,100%%]: %d basis points", p.Rate)
	}
	return nil
}

func (p PercentDiscount) Apply(base Money) Money {
	return base.Sub(base.MulPercent(p.Rate))
}

// AmountDiscount removes a fixed amount. If Amount exceeds the base, the net is 0.
type AmountDiscount struct {
	Amount Money `json:"amount"`
}

func (AmountDiscount) isDiscount() {}

func (a AmountDiscount) Validate() error {
	if a.Amount < 0 {
		return fmt.Errorf("negative discount amount: %s", a.Amount.Format2DP())
	}
	return nil
}

func (a AmountDiscount) Apply(base Money) Money {
	if a.Amount > base {
		return 0
	}
	return base.Sub(a.Amount)
}

// applyDiscount dispatches Apply, treating a nil Discount as the identity.
func applyDiscount(d Discount, base Money) Money {
	if d == nil {
		return base
	}
	return d.Apply(base)
}

func NewPercentDiscount(value float64) (Discount, error) {
	p, err := NewPercent(value)
	if err != nil {
		return nil, err
	}
	return PercentDiscount{Rate: p}, nil
}

func NewAmountDiscount(m Money) (Discount, error) {
	d := AmountDiscount{Amount: m}
	return d, d.Validate()
}

type discountKind string

const (
	discountKindPercent discountKind = "percent"
	discountKindAmount  discountKind = "amount"
)

func (p PercentDiscount) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    discountKind `json:"type"`
		Percent Percent      `json:"percent"`
	}{discountKindPercent, p.Rate})
}

func (a AmountDiscount) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   discountKind `json:"type"`
		Amount Money        `json:"amount"`
	}{discountKindAmount, a.Amount})
}

func unmarshalDiscount(data []byte) (Discount, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var head struct {
		Type discountKind `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	switch head.Type {
	case "":
		return nil, nil
	case discountKindPercent:
		return decodeValidated[PercentDiscount](data)
	case discountKindAmount:
		return decodeValidated[AmountDiscount](data)
	default:
		return nil, fmt.Errorf("invalid discount type: %q", head.Type)
	}
}
