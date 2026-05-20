package domain

import (
	"fmt"
	"math/big"
)

type DiscountKind string

const (
	DiscountNone    DiscountKind = ""
	DiscountPercent DiscountKind = "percent"
	DiscountAmount  DiscountKind = "amount"
)

type Percent struct {
	Value int64
}

const maxPercent = 100 * moneyScale

func ParsePercent(s string) (Percent, error) {
	v, err := parseFixed(s, 5)
	if err != nil {
		return Percent{}, err
	}
	return Percent{Value: v}, nil
}

func (p Percent) String() string { return formatFixed5(p.Value) }

func (p Percent) MarshalJSON() ([]byte, error) {
	return []byte(`"` + p.String() + `"`), nil
}

func (p *Percent) UnmarshalJSON(data []byte) error {
	v, err := ParsePercent(unquoteJSONNumber(data))
	if err != nil {
		return err
	}
	*p = v
	return nil
}

type Discount struct {
	Kind    DiscountKind `json:"kind"`
	Percent Percent      `json:"percent"`
	Amount  Money        `json:"amount"`
}

func NewPercentDiscount(p Percent) (Discount, error) {
	if p.Value < 0 || p.Value > maxPercent {
		return Discount{}, fmt.Errorf("percent out of range: %s", p)
	}
	return Discount{Kind: DiscountPercent, Percent: p}, nil
}

func NewAmountDiscount(m Money) (Discount, error) {
	if m.Amount < 0 {
		return Discount{}, fmt.Errorf("negative discount amount")
	}
	return Discount{Kind: DiscountAmount, Amount: m}, nil
}

func (d Discount) Apply(base Money) Money {
	switch d.Kind {
	case DiscountNone:
		return Money{}
	case DiscountPercent:
		prod := new(big.Int).Mul(big.NewInt(base.Amount), big.NewInt(d.Percent.Value))
		denom := new(big.Int).Mul(bigPercent, big.NewInt(moneyScale))
		return Money{Amount: divRoundHalfUp(prod, denom).Int64()}
	case DiscountAmount:
		if d.Amount.Amount > base.Amount {
			return base
		}
		return d.Amount
	}
	panic(fmt.Sprintf("unknown discount kind: %q", d.Kind))
}
