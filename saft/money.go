package saft

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/flyzard/invoicing.v2/domain"
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
