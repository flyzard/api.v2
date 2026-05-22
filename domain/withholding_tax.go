package domain

import "fmt"

// WithholdingType identifies the kind of tax withheld at source.
// SAF-T WithholdingTaxType enum: IRS (personal income), IRC (corporate income), IS (stamp duty).
type WithholdingType string

const (
	WithholdingIRS WithholdingType = "IRS"
	WithholdingIRC WithholdingType = "IRC"
	WithholdingIS  WithholdingType = "IS"
)

func (t WithholdingType) IsValid() bool {
	switch t {
	case WithholdingIRS, WithholdingIRC, WithholdingIS:
		return true
	}
	return false
}

// WithholdingTax is the SAF-T WithholdingTax block attached to invoices and payments.
// Type and Description are optional per XSD; Amount is mandatory.
type WithholdingTax struct {
	Type        WithholdingType `json:"type,omitempty"`
	Description string          `json:"description,omitempty"`
	Amount      Money           `json:"amount"`
}

func (w WithholdingTax) Validate() error {
	if w.Type != "" && !w.Type.IsValid() {
		return fmt.Errorf("invalid withholding type: %s", w.Type)
	}
	if len(w.Description) > MaxLenWithholdingDescription {
		return fmt.Errorf("withholding description exceeds %d chars", MaxLenWithholdingDescription)
	}
	if w.Amount < 0 {
		return fmt.Errorf("negative withholding amount: %s", w.Amount)
	}
	return nil
}
