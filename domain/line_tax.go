package domain

import (
	"encoding/json"
	"fmt"
)

// TaxJurisdiction matches SAF-T TaxCountryRegion: any ISO 3166-1 alpha-2 plus PT-AC, PT-MA.
// Distinct from the narrower TaxRegion enum used by the canonical VAT rate table.
type TaxJurisdiction string

func (j TaxJurisdiction) IsValid() bool {
	if j == "PT-AC" || j == "PT-MA" {
		return true
	}
	if j == "Desconhecido" {
		return false
	}
	return Country(j).IsValid()
}

// NewTaxJurisdiction is the constructor mirror of NewCountry / NewCurrencyCode /
// NewTaxID for use at ingress boundaries that take a raw string.
func NewTaxJurisdiction(s string) (TaxJurisdiction, error) {
	j := TaxJurisdiction(s)
	if !j.IsValid() {
		return "", fmt.Errorf("invalid tax jurisdiction: %q", s)
	}
	return j, nil
}

func (j *TaxJurisdiction) UnmarshalJSON(data []byte) error {
	return unmarshalString(data, NewTaxJurisdiction, j)
}

// LineTax is the sealed sum of the three SAF-T tax shapes that can appear on a document line.
// A nil LineTax means "no tax assigned yet" and is rejected by DocumentLine.Validate.
type LineTax interface {
	Apply(base Money) Money
	Validate() error
	isLineTax()
}

// VATTax is the canonical VAT line tax. ExemptReason is required when the rate is in the
// TaxExempt category.
type VATTax struct {
	Rate         TaxRate `json:"rate"`
	ExemptReason string  `json:"exempt_reason,omitempty"`
}

func (VATTax) isLineTax() {}

func (v VATTax) Apply(base Money) Money { return base.MulPercent(v.Rate.Value) }

func (v VATTax) Validate() error {
	if err := v.Rate.Validate(); err != nil {
		return err
	}
	if v.Rate.Category == TaxExempt {
		if n := len(v.ExemptReason); n < MinLenExemptReason || n > MaxLenExemptReason {
			return fmt.Errorf("exempt reason text must be %d..%d chars, got %d", MinLenExemptReason, MaxLenExemptReason, n)
		}
		if err := enforceWindows1252(v.ExemptReason, "vat_tax.exempt_reason"); err != nil {
			return err
		}
	}
	return nil
}

// StampTax (Imposto do Selo) is computed as a fixed amount per line; the base is ignored.
type StampTax struct {
	Jurisdiction TaxJurisdiction `json:"jurisdiction"`
	Code         string          `json:"code"`
	Amount       Money           `json:"amount"`
}

func (StampTax) isLineTax() {}

func (s StampTax) Apply(base Money) Money { return s.Amount }

func (s StampTax) Validate() error {
	if !s.Jurisdiction.IsValid() {
		return fmt.Errorf("invalid jurisdiction: %q", s.Jurisdiction)
	}
	if s.Code == "" {
		return fmt.Errorf("stamp code is required")
	}
	if n := len(s.Code); n > MaxLenStampTaxCode {
		return fmt.Errorf("stamp code exceeds %d chars: %q", MaxLenStampTaxCode, s.Code)
	}
	if s.Amount < 0 {
		return fmt.Errorf("negative stamp amount: %d", s.Amount)
	}
	if err := enforceWindows1252(s.Code, "stamp_tax.code"); err != nil {
		return err
	}
	return nil
}

// NotSubjectTax (NS) marks a line as outside the scope of VAT. Reason (Mxx code) and
// ReasonText (6..60 chars) are both mandatory.
type NotSubjectTax struct {
	Jurisdiction TaxJurisdiction `json:"jurisdiction"`
	Reason       Exemption       `json:"reason"`
	ReasonText   string          `json:"reason_text"`
}

func (NotSubjectTax) isLineTax() {}

func (n NotSubjectTax) Apply(base Money) Money { return 0 }

func (n NotSubjectTax) Validate() error {
	if !n.Jurisdiction.IsValid() {
		return fmt.Errorf("invalid jurisdiction: %q", n.Jurisdiction)
	}
	if !n.Reason.Valid() {
		return fmt.Errorf("invalid exemption code: %q", n.Reason)
	}
	if n := len(n.ReasonText); n < MinLenExemptReason || n > MaxLenExemptReason {
		return fmt.Errorf("reason text must be %d..%d chars, got %d", MinLenExemptReason, MaxLenExemptReason, n)
	}
	if err := enforceWindows1252(n.ReasonText, "not_subject_tax.reason_text"); err != nil {
		return err
	}
	return nil
}

func NewVATLineTax(region TaxRegion, category TaxCategory, exemption Exemption, exemptReason string) (LineTax, error) {
	rate, err := GetTaxRate(region, category, exemption)
	if err != nil {
		return nil, err
	}
	t := VATTax{Rate: rate, ExemptReason: exemptReason}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

func NewStampLineTax(jurisdiction TaxJurisdiction, code string, amount Money) (LineTax, error) {
	t := StampTax{Jurisdiction: jurisdiction, Code: code, Amount: amount}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

func NewNotSubjectLineTax(jurisdiction TaxJurisdiction, reason Exemption, reasonText string) (LineTax, error) {
	t := NotSubjectTax{Jurisdiction: jurisdiction, Reason: reason, ReasonText: reasonText}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// lineTaxKind is the JSON discriminator kept in sync with the SAF-T tax-type codes.
type lineTaxKind string

const (
	lineTaxKindVAT   lineTaxKind = "IVA"
	lineTaxKindStamp lineTaxKind = "IS"
	lineTaxKindNS    lineTaxKind = "NS"
)

func (v VATTax) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         lineTaxKind `json:"type"`
		Rate         TaxRate     `json:"rate"`
		ExemptReason string      `json:"exempt_reason,omitempty"`
	}{lineTaxKindVAT, v.Rate, v.ExemptReason})
}

func (s StampTax) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         lineTaxKind     `json:"type"`
		Jurisdiction TaxJurisdiction `json:"jurisdiction"`
		Code         string          `json:"code"`
		Amount       Money           `json:"amount"`
	}{lineTaxKindStamp, s.Jurisdiction, s.Code, s.Amount})
}

func (n NotSubjectTax) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         lineTaxKind     `json:"type"`
		Jurisdiction TaxJurisdiction `json:"jurisdiction"`
		Reason       Exemption       `json:"reason"`
		ReasonText   string          `json:"reason_text"`
	}{lineTaxKindNS, n.Jurisdiction, n.Reason, n.ReasonText})
}

// decodeLineTax unmarshals into a concrete variant T and runs its Validate; the
// caller selects T via the discriminator. Generic so each new variant adds one
// case in unmarshalLineTax instead of a 5-line block.
func decodeLineTax[T LineTax](data []byte) (LineTax, error) {
	var t T
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// unmarshalLineTax picks the concrete variant from the type discriminator.
// Empty / null / missing payload returns (nil, nil); the parent decides whether nil is legal.
func unmarshalLineTax(data []byte) (LineTax, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var head struct {
		Type lineTaxKind `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	switch head.Type {
	case "":
		return nil, nil
	case lineTaxKindVAT:
		return decodeLineTax[VATTax](data)
	case lineTaxKindStamp:
		return decodeLineTax[StampTax](data)
	case lineTaxKindNS:
		return decodeLineTax[NotSubjectTax](data)
	default:
		return nil, fmt.Errorf("invalid tax type: %q", head.Type)
	}
}
