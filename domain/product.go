package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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
	UnitPiece   UnitOfMeasure = "UN" // Unidade
	UnitKg      UnitOfMeasure = "KG" // Quilograma
	UnitGram    UnitOfMeasure = "GR" // Grama
	UnitLiter   UnitOfMeasure = "LT" // Litro
	UnitMeter   UnitOfMeasure = "MT" // Metro
	UnitM2      UnitOfMeasure = "M2" // Metro quadrado
	UnitM3      UnitOfMeasure = "M3" // Metro cúbico
	UnitHour    UnitOfMeasure = "HR" // Hora
	UnitDay     UnitOfMeasure = "DI" // Dia
	UnitMonth   UnitOfMeasure = "MS" // Mês
	UnitPack    UnitOfMeasure = "PC" // Embalagem
	UnitService UnitOfMeasure = "SV" // Serviço
)

func (u UnitOfMeasure) IsValid() bool {
	switch u {
	case UnitPiece, UnitKg, UnitGram, UnitLiter, UnitMeter, UnitM2, UnitM3,
		UnitHour, UnitDay, UnitMonth, UnitPack, UnitService:
		return true
	}
	return false
}

type Product struct {
	ID          uuid.UUID     `json:"id"`
	Code        string        `json:"code,omitempty"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	EAN         string        `json:"ean,omitempty"`
	Type        ProductType   `json:"type"`
	Unit        UnitOfMeasure `json:"unit,omitempty"`
	Price       Money         `json:"price,omitempty"`
	Active      bool          `json:"active"`
}

type ProductOption func(*Product)

func WithCode(s string) ProductOption           { return func(p *Product) { p.Code = s } }
func WithDescription(s string) ProductOption    { return func(p *Product) { p.Description = s } }
func WithEAN(s string) ProductOption            { return func(p *Product) { p.EAN = s } }
func WithUnit(u UnitOfMeasure) ProductOption    { return func(p *Product) { p.Unit = u } }
func WithPrice(m Money) ProductOption           { return func(p *Product) { p.Price = m } }
func WithProductActive(active bool) ProductOption {
	return func(p *Product) { p.Active = active }
}

func NewProduct(name string, productType ProductType, opts ...ProductOption) (Product, error) {
	p := Product{
		ID:     uuid.New(),
		Name:   strings.TrimSpace(name),
		Type:   productType,
		Active: true,
	}
	for _, opt := range opts {
		opt(&p)
	}
	if err := p.Validate(); err != nil {
		return Product{}, err
	}
	return p, nil
}

func (p Product) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("product name is required")
	}
	if !p.Type.IsValid() {
		return fmt.Errorf("invalid product type: %q", p.Type)
	}
	if p.Unit != "" && !p.Unit.IsValid() {
		return fmt.Errorf("invalid unit of measure: %q", p.Unit)
	}
	return nil
}
