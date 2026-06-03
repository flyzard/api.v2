package domain

import (
	"strings"

	"github.com/google/uuid"
)

type ProductType string

// SAF-T (PT) ProductType enum per Portaria 302/2016 + 2025 codes:
//   P — Products (goods)
//   S — Services
//   O — Other (charges/non-product line items)
//   E — Excise duties (IEC) lines
//   I — Parafiscal taxes / charges (taxas)
const (
	ProductTypeGoods      ProductType = "P"
	ProductTypeService    ProductType = "S"
	ProductTypeOther      ProductType = "O"
	ProductTypeExcise     ProductType = "E"
	ProductTypeParafiscal ProductType = "I"
)

func (t ProductType) IsValid() bool {
	switch t {
	case ProductTypeGoods, ProductTypeService, ProductTypeOther, ProductTypeExcise, ProductTypeParafiscal:
		return true
	}
	return false
}

// UnitOfMeasure is the SAF-T UnitOfMeasure field: free text up to 20 chars.
// The constants below are common suggestions; any 1..20 char string is acceptable
// per XSD SAFPTtextTypeMandatoryMax20Car.
type UnitOfMeasure string

const (
	UnitPiece   UnitOfMeasure = "UN"
	UnitKg      UnitOfMeasure = "KG"
	UnitGram    UnitOfMeasure = "GR"
	UnitLiter   UnitOfMeasure = "LT"
	UnitMeter   UnitOfMeasure = "MT"
	UnitM2      UnitOfMeasure = "M2"
	UnitM3      UnitOfMeasure = "M3"
	UnitHour    UnitOfMeasure = "HR"
	UnitDay     UnitOfMeasure = "DI"
	UnitMonth   UnitOfMeasure = "MS"
	UnitPack    UnitOfMeasure = "PC"
	UnitService UnitOfMeasure = "SV"
)

func (u UnitOfMeasure) Validate() error {
	if n := len(u); n < 1 || n > 20 {
		return ErrInvalidUnit
	}
	return nil
}

// NewUnitOfMeasure wraps a string after enforcing the 1..20 length bound.
func NewUnitOfMeasure(s string) (UnitOfMeasure, error) {
	u := UnitOfMeasure(s)
	if err := u.Validate(); err != nil {
		return "", err
	}
	return u, nil
}

func (u *UnitOfMeasure) UnmarshalJSON(data []byte) error {
	// Optional field: a JSON empty-string round-trips as the zero value.
	if string(data) == `""` {
		return nil
	}
	return unmarshalString(data, NewUnitOfMeasure, u)
}

type Product struct {
	ProductID          uuid.UUID     `json:"id"`
	ProductCode        string        `json:"product_code"`
	ProductType        ProductType   `json:"type"`
	ProductGroup       string        `json:"group,omitempty"`
	ProductDescription string        `json:"description"`
	ProductNumberCode  string        `json:"number_code"`
	CustomsDetails     string        `json:"customs_details,omitempty"`
	Unit               UnitOfMeasure `json:"unit,omitempty"`
	Price              Money         `json:"price,omitempty"`
	Active             bool          `json:"active"`
}

// NewProduct fills in zero-value defaults (ID) and validates required fields.
// Callers populate the rest via struct literal — there's no builder.
func NewProduct(p Product) (Product, error) {
	if p.ProductID == uuid.Nil {
		p.ProductID = uuid.New()
	}
	p.ProductCode = strings.TrimSpace(p.ProductCode)
	p.ProductDescription = strings.TrimSpace(p.ProductDescription)
	p.ProductNumberCode = strings.TrimSpace(p.ProductNumberCode)
	if p.ProductCode == "" {
		return Product{}, ErrMissingProductCode
	}
	if !p.ProductType.IsValid() {
		return Product{}, ErrInvalidProductType
	}
	if p.ProductDescription == "" {
		return Product{}, ErrMissingProductDescription
	}
	if p.ProductNumberCode == "" {
		return Product{}, ErrMissingProductNumberCode
	}
	if p.Unit != "" {
		if err := p.Unit.Validate(); err != nil {
			return Product{}, err
		}
	}
	return p, nil
}
