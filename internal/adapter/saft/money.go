package saft

import (
	"encoding/xml"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// saftMoney renders domain.Money at 2dp for totals-level fields. Line-level
// fields use saftMoneyLine (5dp) — totals truncation drift would break
// Σ-line reconciliation (AT cert §5.7). We wrap because domain.Money's
// stdlib JSON marshalling emits integer cents.
type saftMoney domain.Money

func (m saftMoney) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	return enc.EncodeElement(domain.Money(m).Format2DP(), start)
}

// saftMoneyLine renders domain.Money at 5dp (native scale) for line-level
// fields. AT cert §5.7 example: <UnitPrice>0.45144</UnitPrice>.
type saftMoneyLine domain.Money

func (m saftMoneyLine) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	return enc.EncodeElement(format5DP(int64(m)), start)
}

// format5DP renders a scale-100_000 int64 as a 5dp decimal string. Computed
// from the scaled int to avoid float round-trip drift in AT signatures.
func format5DP(scaled int64) string {
	sign := ""
	if scaled < 0 {
		sign = "-"
		scaled = -scaled
	}
	return fmt.Sprintf("%s%d.%05d", sign, scaled/quantityScale, scaled%quantityScale)
}

// saftQty renders domain.Quantity, trimming trailing zeros so whole values
// emit as integers (AT §5.7: <Quantity>100</Quantity>).
type saftQty domain.Quantity

func (q saftQty) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	return enc.EncodeElement(formatQty(int64(q)), start)
}

func formatQty(scaled int64) string {
	if scaled%quantityScale == 0 {
		return strconv.FormatInt(scaled/quantityScale, 10)
	}
	return strings.TrimRight(format5DP(scaled), "0")
}

// quantityScale mirrors domain.scale (100_000). Kept local so saft does not
// depend on an unexported domain constant.
const quantityScale = 100_000

// fmtPercent renders domain.Percent as fixed 2-decimal text ("23.00").
func fmtPercent(p domain.Percent) string {
	return fmt.Sprintf("%.2f", float64(p)/100)
}

// linePricePairK is the number of extra decimal places used for UnitPrice
// above the native scale. Total UnitPrice precision = 5 + linePricePairK dp.
// K=10 gives 15 fractional digits, well within the power-of-10 denominator
// (10^15) that keeps CreditAmount a terminating decimal. Larger K improves
// the approximation to the true UnitPrice but increases output width.
const linePricePairK = 10

// linePricePair returns matched (unitPriceStr, amountStr) such that
// parseDecimal(unitPriceStr) × parseDecimal(qty rendered by formatQty) ==
// parseDecimal(amountStr) exactly. This satisfies AT's identity requirement
// "Quantity × UnitPrice = CreditAmount/DebitAmount, não devendo ser arredondado."
//
// Algorithm:
//
//   - Both net (domain.Money) and qty (domain.Quantity) share scale=100_000
//     (internal units), so UnitPrice in EUR = net_int / qty_int (scale cancels).
//
//   - Render UnitPrice at K decimal places:
//     up_int = round(net_int × 10^K / qty_int)
//     UnitPrice = up_int / 10^K   (denominator = 10^K, a pure power of 10)
//
//   - CreditAmount = exact decimal product of the rendered Quantity × UnitPrice:
//     qty_EUR = qty_int / quantityScale
//     product  = (qty_int / quantityScale) × (up_int / 10^K)
//     = qty_int × up_int / (quantityScale × 10^K)
//     The denominator quantityScale × 10^K = 10^(5+K) is a pure power of 10,
//     so the product always terminates in at most (5+K) fractional digits.
//
//   - Both strings are trimmed of trailing zeros to a minimum of 5 dp so
//     whole-quantity lines keep the traditional "50.00000" shape.
//
// qty == 0 is not valid (domain enforces qty > 0); callers must ensure qty > 0.
func linePricePair(net domain.Money, qty domain.Quantity) (unitPriceStr, amountStr string) {
	// 10^K multiplier for UnitPrice precision.
	kMul := new(big.Int).Exp(big.NewInt(10), big.NewInt(linePricePairK), nil)

	netInt := big.NewInt(int64(net))
	qtyInt := big.NewInt(int64(qty))

	// up_int = round(net_int × 10^K / qty_int), half-away-from-zero.
	// round(a/b) = floor((a + b/2) / b) for a,b > 0.
	num := new(big.Int).Mul(netInt, kMul)
	half := new(big.Int).Rsh(qtyInt, 1) // qty_int / 2
	num.Add(num, half)
	upInt := new(big.Int).Quo(num, qtyInt)

	// UnitPrice denominator = 10^K.
	upDenom := new(big.Int).Set(kMul)

	// CreditAmount = qty_int × up_int / (quantityScale × 10^K).
	prodNum := new(big.Int).Mul(qtyInt, upInt)
	prodDenom := new(big.Int).Mul(big.NewInt(quantityScale), kMul) // 10^(5+K)

	unitPriceStr = formatBigDecimal(upInt, upDenom)
	amountStr = formatBigDecimal(prodNum, prodDenom)
	return
}

// formatBigDecimal renders numerator/denominator as a decimal string trimmed
// to at minimum 5 decimal places. The denominator must be a positive power of 10.
func formatBigDecimal(num, denom *big.Int) string {
	// Determine the number of decimal places = log10(denom).
	dp := 0
	for d := new(big.Int).Set(denom); d.Cmp(big.NewInt(1)) > 0; {
		d.Quo(d, big.NewInt(10))
		dp++
	}
	// Integer and fractional parts.
	intPart := new(big.Int)
	fracPart := new(big.Int)
	intPart.QuoRem(num, denom, fracPart)

	fracStr := fmt.Sprintf("%0*d", dp, fracPart.Int64())

	// Trim trailing zeros but keep at least 5 decimal places.
	minDP := 5
	trimTo := len(fracStr)
	for trimTo > minDP && fracStr[trimTo-1] == '0' {
		trimTo--
	}
	return fmt.Sprintf("%d.%s", intPart, fracStr[:trimTo])
}
