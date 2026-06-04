package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"time"
)

// CurrencyCode is an ISO 4217 alphabetic code.
type CurrencyCode string

// currencyCodePattern mirrors the XSD CurrencyCode restriction (EUR intentionally omitted).
var currencyCodePattern = regexp.MustCompile(
	`^(AED|AFN|ALL|AMD|ANG|AOA|ARS|AUD|AWG|AZN|BAM|BBD|BDT|BGN|BHD|BIF|BMD|BND|BOB|BOV|BRL|BSD|BTN|BWP|BYN|BYR|BZD|CAD|CDF|CHE|CHF|CHW|CLF|CLP|CNY|COP|COU|CRC|CUC|CUP|CVE|CZK|DJF|DKK|DOP|DZD|EGP|ERN|ETB|FJD|FKP|GBP|GEL|GHS|GIP|GMD|GNF|GTQ|GYD|HKD|HNL|HRK|HTG|HUF|IDR|ILS|INR|IQD|IRR|ISK|JMD|JOD|JPY|KES|KGS|KHR|KMF|KPW|KRW|KWD|KYD|KZT|LAK|LBP|LKR|LRD|LSL|LTL|LVL|LYD|MAD|MDL|MGA|MKD|MMK|MNT|MOP|MRO|MRU|MUR|MVR|MWK|MXN|MXV|MYR|MZN|NAD|NGN|NIO|NOK|NPR|NZD|OMR|PAB|PEN|PGK|PHP|PKR|PLN|PYG|QAR|RON|RSD|RUB|RWF|SAR|SBD|SCR|SDG|SEK|SGD|SHP|SLE|SLL|SOS|SRD|SSP|STD|STN|SVC|SYP|SZL|THB|TJS|TMT|TND|TOP|TRY|TTD|TWD|TZS|UAH|UGX|USD|USN|USS|UYI|UYU|UZS|VED|VEF|VES|VND|VUV|WST|XAF|XAG|XAU|XBA|XBB|XBC|XBD|XCD|XDR|XFU|XOF|XPD|XPF|XPT|XSU|XUA|YER|ZAR|ZMW|ZWL|EEK|SKK|TMM|ZMK|ZWD|ZWR)$`,
)

func (c CurrencyCode) IsValid() bool { return currencyCodePattern.MatchString(string(c)) }

// NewCurrencyCode wraps a string in CurrencyCode after validating
func NewCurrencyCode(s string) (CurrencyCode, error) {
	c := CurrencyCode(s)
	if !c.IsValid() {
		return "", fmt.Errorf("invalid currency code: %q", s)
	}
	return c, nil
}

func (c *CurrencyCode) UnmarshalJSON(data []byte) error {
	return unmarshalString(data, NewCurrencyCode, c)
}

// ExchangeRate uses 6-decimal scale to fit both strong (USD≈0.92/EUR) and weak (JPY≈170/EUR) currencies without precision loss.
type ExchangeRate int64

const exchangeRateScale = 1_000_000

// NewExchangeRate takes a rate (e.g. 1.085 for USD per EUR) and returns scaled.
// Zero or negative rates are rejected; XSD allows 0 via SAFdecimalType but it has no business meaning.
func NewExchangeRate(rate float64) (ExchangeRate, error) {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("invalid exchange rate: %v", rate)
	}
	if rate <= 0 {
		return 0, fmt.Errorf("non-positive exchange rate: %v", rate)
	}
	scaled := math.Round(rate * exchangeRateScale)
	if scaled > maxScaled {
		return 0, fmt.Errorf("exchange rate overflows int64: %v", rate)
	}
	return ExchangeRate(scaled), nil
}

func (r ExchangeRate) Float64() float64 { return float64(r) / exchangeRateScale }

func (r ExchangeRate) MarshalJSON() ([]byte, error) { return json.Marshal(r.Float64()) }

func (r *ExchangeRate) UnmarshalJSON(data []byte) error {
	return unmarshalFloat(data, NewExchangeRate, r)
}

// Currency is the foreign-currency view of document totals: domain Money stays
// in EUR and this block tells the projector how to render it natively. Date
// must equal the document's InvoiceDate so the rate cannot drift between draft
// prep and issuance. Amount is EUR-equivalent at cent precision; the projector
// reconstructs native-precision amounts from Amount × ExchangeRate at export.
// TODO(M-1): introduce ForeignAmount{Cents, Precision} only when a JPY/KWD-
// precision consumer actually needs it.
type Currency struct {
	Code         CurrencyCode `json:"code"`
	Amount       Money        `json:"amount"`
	ExchangeRate ExchangeRate `json:"exchange_rate"`
	Date         time.Time    `json:"date"`
}

func NewCurrency(code CurrencyCode, amount Money, rate ExchangeRate, date time.Time) (Currency, error) {
	c := Currency{Code: code, Amount: amount, ExchangeRate: rate, Date: date}
	return c, c.Validate()
}

func (c Currency) Validate() error {
	if !c.Code.IsValid() {
		return fmt.Errorf("invalid currency code: %q", c.Code)
	}
	if c.Amount <= 0 {
		return fmt.Errorf("currency amount must be positive: %d", c.Amount)
	}
	if c.ExchangeRate <= 0 {
		return fmt.Errorf("exchange rate must be positive: %d", c.ExchangeRate)
	}
	if c.Date.IsZero() {
		return fmt.Errorf("currency date is required")
	}
	return nil
}
