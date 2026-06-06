package domain

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strconv"
)

// Money is always EUR. For FX, see Currency.
// Stored as scaled int64. 1 EUR = scale (100_000). 5-decimal precision.
type Money int64

// Quantity shares scale with Money so Mul math divides cleanly.
type Quantity int64

// scale applies to both Money and Quantity. Percent uses its own scale; see percentage.go.
const scale = 100_000

// centScale is the internal-units-per-cent ratio (scale / 100 = 1_000).
// NewMoney rejects any input whose scaled value is not a clean multiple of this,
// since AT requires cent precision on every monetary input.
const centScale = scale / 100

// maxScaled bounds scaled values safely convertible to int64.
// float64(math.MaxInt64) rounds up to 2^63 (would wrap on conversion);
// Nextafter steps to the largest float strictly below 2^63.
var maxScaled = math.Nextafter(float64(math.MaxInt64), 0)

// NewMoney takes euros (1.50 = €1.50) and returns scaled Money.
// Inputs with sub-cent precision (e.g. 0.005) are rejected with ErrSubCentPrecision:
// AT's SAF-T export and signing path both demand 2-decimal precision (Portaria 363/2010,
// regras §R-G3 / I-F4). Sub-cent intermediates produced by Mul / MulPercent are still
// fine — only constructor inputs are gated.
func NewMoney(euros float64) (Money, error) {
	if math.IsNaN(euros) || math.IsInf(euros, 0) {
		return 0, fmt.Errorf("invalid money: %v", euros)
	}
	scaled := math.Round(euros * scale)
	if scaled > maxScaled || scaled < math.MinInt64 {
		return 0, fmt.Errorf("money overflows int64: %v", euros)
	}
	if int64(scaled)%centScale != 0 {
		return 0, fmt.Errorf("%w: %v", ErrSubCentPrecision, euros)
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
// Panics on int64 overflow (~€920M × ~1e10 units). The threshold is far above
// any realistic invoice — the panic exists to surface programmer error if the
// scale design ever changes underneath.
func (m Money) Mul(qty Quantity) Money {
	mi, qi := int64(m), int64(qty)
	if mi != 0 && qi != 0 {
		am, aq := abs64(mi), abs64(qi)
		if aq > math.MaxInt64/am {
			panic(fmt.Sprintf("Money.Mul overflow: %d × %d", mi, qi))
		}
	}
	return Money(roundDiv(mi*qi, scale))
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
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

// prorateCents splits a whole-cent amount across weights proportionally using
// the largest-remainder method (ties to the lower index). Results are Money
// (scaled units), each a whole cent, summing to exactly cents*centScale.
// Whole-cent shares keep Money's integer-cents JSON contract lossless.
// weights must be non-negative with a positive sum; cents must satisfy
// cents*centScale <= Σweights (callers derive cents from a discount bounded
// by the weight sum). 128-bit intermediates: cents×weight overflows int64
// for documents in the millions of euros.
func prorateCents(cents int64, weights []Money) []Money {
	var sum uint64
	for _, w := range weights {
		sum += uint64(w)
	}
	type rem struct {
		idx int
		r   uint64
	}
	shares := make([]Money, len(weights))
	rems := make([]rem, len(weights))
	allocated := int64(0)
	for i, w := range weights {
		hi, lo := bits.Mul64(uint64(cents), uint64(w))
		q, r := bits.Div64(hi, lo, sum) // hi < sum: cents < 2^64 and w <= sum
		shares[i] = Money(int64(q) * centScale)
		allocated += int64(q)
		rems[i] = rem{i, r}
	}
	slices.SortStableFunc(rems, func(a, b rem) int {
		return cmp.Compare(b.r, a.r) // descending remainder; stable sort keeps lower index first on ties
	})
	for k := int64(0); k < cents-allocated; k++ {
		shares[rems[k].idx] += Money(centScale)
	}
	return shares
}

// Float64 returns the euro value as float64. Lossy; use for display only.
func (m Money) Float64() float64 {
	return float64(m) / scale
}

func (m Money) String() string {
	return fmt.Sprintf("€%.5f", m.Float64())
}

// Format2DP renders Money as euros at 2 decimal places ("123.45"),
// rounding half-away-from-zero. Computed from the scaled int64 to avoid
// float round-trip drift in AT signatures.
func (m Money) Format2DP() string {
	cents := roundDiv(int64(m), scale/100)
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
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

// unmarshalString is the string-typed sibling of unmarshalFloat.
// Shared by Country/CurrencyCode/TaxID/UnitOfMeasure UnmarshalJSON.
func unmarshalString[T any](data []byte, ctor func(string) (T, error), out *T) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	x, err := ctor(s)
	if err != nil {
		return err
	}
	*out = x
	return nil
}

// MarshalJSON emits Money as integer cents to match the AT 2-decimal contract
// and to keep the wire format free of float round-trip drift. €49.50 → 4950.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(m) / centScale)
}

func (m *Money) UnmarshalJSON(data []byte) error {
	var cents int64
	if err := json.Unmarshal(data, &cents); err != nil {
		return err
	}
	*m = Money(cents * centScale)
	return nil
}
func (q Quantity) MarshalJSON() ([]byte, error)     { return json.Marshal(float64(q) / scale) }
func (q *Quantity) UnmarshalJSON(data []byte) error { return unmarshalFloat(data, NewQuantity, q) }

// String renders the quantity with its natural number of decimals ("2.5", "10").
func (q Quantity) String() string {
	return strconv.FormatFloat(float64(q)/scale, 'f', -1, 64)
}
