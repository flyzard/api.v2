package domain

import "github.com/google/uuid"

type ProductType string

const (
	ProductTypeGoods   ProductType = "P"
	ProductTypeService ProductType = "S"
)

func (t ProductType) IsValid() bool {
	switch t {
	case ProductTypeGoods, ProductTypeService:
		return true
	}
	return false
}

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

var validUnits = map[UnitOfMeasure]struct{}{
	UnitPiece: {}, UnitKg: {}, UnitGram: {}, UnitLiter: {}, UnitMeter: {},
	UnitM2: {}, UnitM3: {}, UnitHour: {}, UnitDay: {}, UnitMonth: {},
	UnitPack: {}, UnitService: {},
}

func (u UnitOfMeasure) IsValid() bool {
	_, ok := validUnits[u]
	return ok
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

type ProductOption func(*Product)

func WithProductGroup(s string) ProductOption   { return func(p *Product) { p.ProductGroup = s } }
func WithCustomsDetails(s string) ProductOption { return func(p *Product) { p.CustomsDetails = s } }
func WithUnit(u UnitOfMeasure) ProductOption    { return func(p *Product) { p.Unit = u } }
func WithPrice(m Money) ProductOption           { return func(p *Product) { p.Price = m } }
func WithProductActive(b bool) ProductOption    { return func(p *Product) { p.Active = b } }

func NewProduct(code string, productType ProductType, description, numberCode string, opts ...ProductOption) (Product, error) {
	if code == "" {
		return Product{}, ErrMissingProductCode
	}
	if !productType.IsValid() {
		return Product{}, ErrInvalidProductType
	}
	if description == "" {
		return Product{}, ErrMissingProductDescription
	}
	if numberCode == "" {
		return Product{}, ErrMissingProductNumberCode
	}
	p := Product{
		ProductID:          uuid.New(),
		ProductCode:        code,
		ProductType:        productType,
		ProductDescription: description,
		ProductNumberCode:  numberCode,
		Active:             true,
	}
	for _, opt := range opts {
		opt(&p)
	}
	if p.Unit != "" && !p.Unit.IsValid() {
		return Product{}, ErrInvalidUnit
	}
	return p, nil
}
