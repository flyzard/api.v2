package domain

import "slices"

// Deep-copy helpers used at issuance: issued documents are immutable, so they
// must not share slice backing arrays or pointees with the caller's draft —
// a post-issue draft edit would otherwise silently rewrite the signed record.

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneLines(lines []DocumentLine) []DocumentLine {
	out := slices.Clone(lines)
	for i := range out {
		out[i].TaxBase = clonePtr(out[i].TaxBase)
		out[i].OrderReferences = slices.Clone(out[i].OrderReferences)
		for j := range out[i].OrderReferences {
			out[i].OrderReferences[j].OrderDate = clonePtr(out[i].OrderReferences[j].OrderDate)
		}
		out[i].References = slices.Clone(out[i].References)
		out[i].SerialNumbers = slices.Clone(out[i].SerialNumbers)
	}
	return out
}

func cloneShippingPoint(p *ShippingPoint) *ShippingPoint {
	out := clonePtr(p)
	if out != nil {
		out.DeliveryIDs = slices.Clone(out.DeliveryIDs)
		out.DeliveryDate = clonePtr(out.DeliveryDate)
		out.Address = clonePtr(out.Address)
	}
	return out
}

func (c Customer) clone() Customer {
	c.ShipToAddresses = slices.Clone(c.ShipToAddresses)
	return c
}

func clonePaymentLines(lines []PaymentLine) []PaymentLine {
	out := slices.Clone(lines)
	for i := range out {
		out[i].SourceDocuments = slices.Clone(out[i].SourceDocuments)
		out[i].SettlementAmount = clonePtr(out[i].SettlementAmount)
	}
	return out
}

func (f SalesInvoiceFields) clone() SalesInvoiceFields {
	f.ShipTo = cloneShippingPoint(f.ShipTo)
	f.ShipFrom = cloneShippingPoint(f.ShipFrom)
	f.MovementStartTime = clonePtr(f.MovementStartTime)
	f.MovementEndTime = clonePtr(f.MovementEndTime)
	f.WithholdingTax = slices.Clone(f.WithholdingTax)
	f.Payments = slices.Clone(f.Payments)
	f.Currency = clonePtr(f.Currency)
	return f
}

func (f StockMovementFields) clone() StockMovementFields {
	f.MovementEndTime = clonePtr(f.MovementEndTime)
	f.ShipFrom = cloneShippingPoint(f.ShipFrom)
	f.ShipTo = cloneShippingPoint(f.ShipTo)
	return f
}
