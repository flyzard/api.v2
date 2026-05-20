package domain

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type Money struct {
	Amount int64
}

const (
	moneyScale   = 100000
	centsDivisor = 1000
)

var bigPercent = big.NewInt(100)

func NewMoneyFromCents(cents int64) Money {
	return Money{Amount: cents * centsDivisor}
}

func ParseMoney(s string) (Money, error) {
	amt, err := parseFixed(s, 5)
	if err != nil {
		return Money{}, err
	}
	return Money{Amount: amt}, nil
}

func (m Money) String() string { return formatFixed5(m.Amount) }

func (m Money) Add(o Money) Money { return Money{m.Amount + o.Amount} }
func (m Money) Sub(o Money) Money { return Money{m.Amount - o.Amount} }
func (m Money) Neg() Money        { return Money{-m.Amount} }

func (m Money) Mul(qty Quantity) Money {
	prod := new(big.Int).Mul(big.NewInt(m.Amount), big.NewInt(int64(qty)))
	return Money{Amount: divRoundHalfUp(prod, big.NewInt(moneyScale)).Int64()}
}

func (m Money) MulPercent(pct int) Money {
	prod := new(big.Int).Mul(big.NewInt(m.Amount), big.NewInt(int64(pct)))
	return Money{Amount: divRoundHalfUp(prod, bigPercent).Int64()}
}

func (m Money) RoundToCents() Money {
	return Money{Amount: m.Cents() * centsDivisor}
}

func (m Money) Cents() int64 {
	half := int64(centsDivisor / 2)
	if m.Amount >= 0 {
		return (m.Amount + half) / centsDivisor
	}
	return -((-m.Amount + half) / centsDivisor)
}

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}

func (m *Money) UnmarshalJSON(data []byte) error {
	v, err := ParseMoney(unquoteJSONNumber(data))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

func parseFixed(s string, decimals int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty value")
	}

	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}

	intPart, fracPart, hasDot := s, "", false
	if i := strings.IndexByte(s, '.'); i >= 0 {
		hasDot = true
		intPart, fracPart = s[:i], s[i+1:]
	}
	if intPart == "" && fracPart == "" {
		return 0, fmt.Errorf("invalid number: %q", s)
	}
	if hasDot && fracPart == "" {
		return 0, fmt.Errorf("trailing dot not allowed: %q", s)
	}
	if !isDigits(intPart) || !isDigits(fracPart) {
		return 0, fmt.Errorf("invalid number: %q", s)
	}
	if hasDot && len(fracPart) > decimals {
		return 0, fmt.Errorf("too many decimal places (max %d): %q", decimals, s)
	}
	for len(fracPart) < decimals {
		fracPart += "0"
	}
	if intPart == "" {
		intPart = "0"
	}
	v, err := strconv.ParseInt(intPart+fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("number out of range: %q", s)
	}
	if neg {
		v = -v
	}
	return v, nil
}

func formatFixed5(v int64) string {
	sign := ""
	if v < 0 {
		v, sign = -v, "-"
	}
	return fmt.Sprintf("%s%d.%05d", sign, v/moneyScale, v%moneyScale)
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func unquoteJSONNumber(data []byte) string {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func divRoundHalfUp(num, den *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	twoRem := new(big.Int).Abs(r)
	twoRem.Lsh(twoRem, 1)
	if twoRem.Cmp(den) >= 0 {
		if num.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}
