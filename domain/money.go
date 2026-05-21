package domain

import (
	"encoding/json"
	"fmt"
	"math"
)

// Money is always EUR. For FX, see Currency.
// Stored as scaled int64. 1 EUR = scale (100_000). 5-decimal precision.
type Money int64

// Quantity shares scale with Money so Mul math divides cleanly.
type Quantity int64

// scale applies to both Money and Quantity. Percent uses its own scale; see percentage.go.
const scale = 100_000

// maxScaled bounds scaled values safely convertible to int64.
// float64(math.MaxInt64) rounds up to 2^63 (would wrap on conversion);
// Nextafter steps to the largest float strictly below 2^63.
var maxScaled = math.Nextafter(float64(math.MaxInt64), 0)

// NewMoney takes euros (1.50 = €1.50) and returns scaled Money.
func NewMoney(euros float64) (Money, error) {
	if math.IsNaN(euros) || math.IsInf(euros, 0) {
		return 0, fmt.Errorf("invalid money: %v", euros)
	}
	scaled := math.Round(euros * scale)
	if scaled > maxScaled || scaled < math.MinInt64 {
		return 0, fmt.Errorf("money overflows int64: %v", euros)
	}
	return Money(scaled), nil
}

// NewQuantity takes a quantity value (1.5 = 1.5 units) and returns scaled Quantity.
// Negative quantities are rejected; credit notes are modelled at document level, not line qty.
func NewQuantity(value float64) (Quantity, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid quantity: %v", value)
	}
	if value < 0 {
		return 0, fmt.Errorf("negative quantity: %v", value)
	}
	scaled := math.Round(value * scale)
	if scaled > maxScaled {
		return 0, fmt.Errorf("quantity overflows int64: %v", value)
	}
	return Quantity(scaled), nil
}

func (m Money) Add(o Money) Money { return m + o }
func (m Money) Sub(o Money) Money { return m - o }

// Mul multiplies money by quantity. Rounds half away from zero.
// Overflow risk when price × qty exceeds ~€920M (both operands carry 1e5 scale).
func (m Money) Mul(qty Quantity) Money {
	return Money(roundDiv(int64(m)*int64(qty), scale))
}

// MulPercent applies a percent. Rounds half away from zero. Overflow risk above ~€9B.
// Panics if p is outside [0, PercentScale] — JSON and NewPercent enforce the range,
// so a violation here means a programmatic literal bypassed the constructor.
func (m Money) MulPercent(p Percent) Money {
	if p < 0 || p > PercentScale {
		panic(fmt.Sprintf("invalid percent: %d (must be 0..%d)", p, PercentScale))
	}
	return Money(roundDiv(int64(m)*int64(p), PercentScale))
}

// roundDiv divides with half-away-from-zero rounding. den must be positive.
func roundDiv(num, den int64) int64 {
	if num >= 0 {
		return (num + den/2) / den
	}
	return (num - den/2) / den
}

// Float64 returns the euro value as float64. Lossy; use for display only.
func (m Money) Float64() float64 {
	return float64(m) / scale
}

func (m Money) String() string {
	return fmt.Sprintf("€%.5f", m.Float64())
}

// unmarshalFloat decodes a JSON number, validates via ctor, and assigns the result.
// Shared by Money/Quantity/Percent UnmarshalJSON since they follow the same shape.
func unmarshalFloat[T any](data []byte, ctor func(float64) (T, error), out *T) error {
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	x, err := ctor(v)
	if err != nil {
		return err
	}
	*out = x
	return nil
}

func (m Money) MarshalJSON() ([]byte, error)        { return json.Marshal(m.Float64()) }
func (m *Money) UnmarshalJSON(data []byte) error    { return unmarshalFloat(data, NewMoney, m) }
func (q Quantity) MarshalJSON() ([]byte, error)     { return json.Marshal(float64(q) / scale) }
func (q *Quantity) UnmarshalJSON(data []byte) error { return unmarshalFloat(data, NewQuantity, q) }
