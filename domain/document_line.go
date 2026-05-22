package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// OrderReference points to a customer order or other originating document.
// Both fields are optional in XSD; if used, OriginatingON is ≤60 chars.
type OrderReference struct {
	OriginatingON string     `json:"originating_on,omitempty"`
	OrderDate     *time.Time `json:"order_date,omitempty"`
}

func (o OrderReference) Validate() error {
	if len(o.OriginatingON) > MaxLenOriginatingON {
		return fmt.Errorf("originating_on exceeds %d chars: %q", MaxLenOriginatingON, o.OriginatingON)
	}
	return enforceWindows1252(o.OriginatingON, "order_reference.originating_on")
}

// DocReference links a credit/debit note line to the invoice line it adjusts.
// AT business rules require References on every NC/ND line; the doc-level enforcement
// is applied when the parent document is issued (Phase 5).
type DocReference struct {
	Reference string `json:"reference,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func (r DocReference) Validate() error {
	if r.Reference == "" {
		return fmt.Errorf("reference is required on DocReference")
	}
	if len(r.Reference) > MaxLenReference {
		return fmt.Errorf("reference exceeds %d chars: %q", MaxLenReference, r.Reference)
	}
	if len(r.Reason) > 50 {
		return fmt.Errorf("reason exceeds 50 chars: %q", r.Reason)
	}
	if err := enforceWindows1252(r.Reference, "doc_reference.reference"); err != nil {
		return err
	}
	return enforceWindows1252(r.Reason, "doc_reference.reason")
}

type DocumentLine struct {
	ID              int              `json:"id"`
	LineNumber      int              `json:"line_number"`
	Product         Product          `json:"product"`
	// Description is the SAF-T Line.Description — frozen at line construction
	// from Product.ProductDescription (Policy B). The Validate gate below requires
	// it to still match the embedded product at issue time, so any out-of-band
	// mutation to either is caught (F-SAFT-9).
	Description     string           `json:"description"`
	Quantity        Quantity         `json:"quantity"`
	UnitPrice       Money            `json:"unit_price"`
	TaxBase         *Money           `json:"tax_base,omitempty"`
	TaxPointDate    time.Time        `json:"tax_point_date"`
	OrderReferences []OrderReference `json:"order_references,omitempty"`
	References      []DocReference   `json:"references,omitempty"`
	SerialNumbers   []string         `json:"serial_numbers,omitempty"`
	Discount        Discount         `json:"discount,omitempty"`
	Tax             LineTax          `json:"tax"`
}


func (l DocumentLine) LineSubtotal() Money {
	return l.UnitPrice.Mul(l.Quantity)
}

// LineTotal = (unit × qty − discount) + tax(after-discount base).
// Tax base is post-discount per PT VAT rules; stamp duty Amount is fixed regardless of base.
// A nil Tax (non-valued transport line) contributes zero tax.
func (l DocumentLine) LineTotal() Money {
	afterDiscount := applyDiscount(l.Discount, l.LineSubtotal())
	if l.Tax == nil {
		return afterDiscount
	}
	return afterDiscount.Add(l.Tax.Apply(afterDiscount))
}

func (l DocumentLine) Validate() error {
	if l.LineNumber < 0 {
		return fmt.Errorf("negative line number: %d", l.LineNumber)
	}
	if l.UnitPrice < 0 {
		return fmt.Errorf("negative unit price: %s", l.UnitPrice)
	}
	if l.Quantity <= 0 {
		return fmt.Errorf("non-positive quantity: %d", l.Quantity)
	}
	if l.TaxPointDate.IsZero() {
		return fmt.Errorf("tax point date is required")
	}
	if n := len(l.Description); n < 1 || n > 200 {
		return fmt.Errorf("description length must be 1..200, got %d", n)
	}
	if err := enforceWindows1252(l.Description, "line.description"); err != nil {
		return err
	}
	if l.Description != l.Product.ProductDescription {
		return fmt.Errorf("line description %q drifts from product description %q (F-SAFT-9)", l.Description, l.Product.ProductDescription)
	}
	// XSD assertion: TaxBase and UnitPrice are mutually exclusive when nonzero.
	if l.TaxBase != nil {
		if *l.TaxBase < 0 {
			return fmt.Errorf("negative tax base: %s", *l.TaxBase)
		}
		if *l.TaxBase > 0 && l.UnitPrice > 0 {
			return fmt.Errorf("tax_base and unit_price cannot both be non-zero")
		}
	}
	if l.Tax != nil {
		if err := l.Tax.Validate(); err != nil {
			return fmt.Errorf("tax: %w", err)
		}
	}
	for i, ref := range l.OrderReferences {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("order_reference[%d]: %w", i, err)
		}
	}
	for i, ref := range l.References {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("reference[%d]: %w", i, err)
		}
	}
	for i, sn := range l.SerialNumbers {
		if sn == "" || len(sn) > 100 {
			return fmt.Errorf("serial_number[%d] length must be 1..100", i)
		}
		if err := enforceWindows1252(sn, fmt.Sprintf("serial_number[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

// UnmarshalJSON peels off the polymorphic Discount and Tax fields as RawMessages
// and dispatches to the per-interface helpers; everything else round-trips through
// the alias.
func (l *DocumentLine) UnmarshalJSON(data []byte) error {
	type alias DocumentLine
	aux := struct {
		*alias
		Discount json.RawMessage `json:"discount,omitempty"`
		Tax      json.RawMessage `json:"tax"`
	}{alias: (*alias)(l)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	d, err := unmarshalDiscount(aux.Discount)
	if err != nil {
		return fmt.Errorf("discount: %w", err)
	}
	l.Discount = d
	tax, err := unmarshalLineTax(aux.Tax)
	if err != nil {
		return fmt.Errorf("tax: %w", err)
	}
	l.Tax = tax
	return l.Validate()
}
