package domain

type DocumentLine struct {
	ID        int16    `json:"id"`
	Product   Product  `json:"product"`
	Quantity  Quantity `json:"quantity"`
	UnitPrice Money    `json:"unit_price"`
	Discount  Discount `json:"discount,omitzero"`
	TaxRate   TaxRate  `json:"tax_rate"`
}

func NewDocumentLine(product Product, quantity Quantity, unitPrice Money, discount Discount, taxRate TaxRate) DocumentLine {
	return DocumentLine{
		ID:        0, // ID will be set when the line is added to a document
		Product:   product,
		Quantity:  quantity,
		UnitPrice: unitPrice,
		Discount:  discount,
		TaxRate:   taxRate,
	}
}

func (l DocumentLine) LineSubtotal() Money {
	return l.UnitPrice.Mul(l.Quantity)
}

func (l DocumentLine) LineTotal() Money {
	lineTotal := l.UnitPrice.Mul(l.Quantity)
	discountAmount := l.Discount.Apply(lineTotal)
	lineTotal = lineTotal.Sub(discountAmount)
	taxAmount := lineTotal.MulPercent(l.TaxRate.Value)
	return lineTotal.Add(taxAmount)
}
