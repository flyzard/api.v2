package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// Percent is stored in basis points: 10000 = 100%, 2300 = 23%, 650 = 6.5%.
// Scale is intentionally separate from Money/Quantity (100_000); see money.go.
type Percent int64

const PercentScale = 10_000

// NewPercent takes a human percent value (23.0 = 23%) and returns it in basis points.
// Range: [0, 100].
func NewPercent(value float64) (Percent, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid percent: %v", value)
	}
	if value < 0 || value > 100 {
		return 0, fmt.Errorf("percent out of range: %v", value)
	}
	return Percent(math.Round(value * PercentScale / 100)), nil
}

func (p Percent) MarshalJSON() ([]byte, error)     { return json.Marshal(float64(p) * 100 / PercentScale) }
func (p *Percent) UnmarshalJSON(data []byte) error { return unmarshalFloat(data, NewPercent, p) }

// Format2DP renders the percentage at 2 decimal places ("23.00") — the
// rendering AT expects for TaxPercentage fields.
func (p Percent) Format2DP() string {
	return strconv.FormatFloat(float64(p)*100/PercentScale, 'f', 2, 64)
}
