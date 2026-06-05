package saft

import "github.com/flyzard/invoicing.v2/internal/domain"

// xmlMovementOfGoods mirrors SAF-T SourceDocuments/MovementOfGoods.
// Family aggregates: total line count and total quantity across every
// movement document.
type xmlMovementOfGoods struct {
	NumberOfMovementLines int                `xml:"NumberOfMovementLines"`
	TotalQuantityIssued   saftQty            `xml:"TotalQuantityIssued"`
	Movements             []xmlStockMovement `xml:"StockMovement"`
}

type xmlStockMovement struct {
	DocumentNumber    string            `xml:"DocumentNumber"`
	ATCUD             string            `xml:"ATCUD"`
	DocumentStatus    xmlMovementStatus `xml:"DocumentStatus"`
	Hash              string            `xml:"Hash"`
	HashControl       string            `xml:"HashControl"`
	MovementDate      string            `xml:"MovementDate"`
	MovementType      string            `xml:"MovementType"`
	SystemEntryDate   string            `xml:"SystemEntryDate"`
	CustomerID        string            `xml:"CustomerID"`
	SourceID          string            `xml:"SourceID"`
	EACCode           string            `xml:"EACCode,omitempty"`
	ShipTo            *xmlShippingPoint `xml:"ShipTo,omitempty"`
	ShipFrom          *xmlShippingPoint `xml:"ShipFrom,omitempty"`
	MovementEndTime   string            `xml:"MovementEndTime,omitempty"`
	MovementStartTime string            `xml:"MovementStartTime"`
	ATDocCodeID       string            `xml:"ATDocCodeID,omitempty"`
	Lines             []xmlMovementLine `xml:"Line"`
	DocumentTotals    xmlSimpleTotals   `xml:"DocumentTotals"`
}

// xmlMovementLine mirrors the MovementOfGoods StockMovement/Line sequence,
// which is narrower than the SalesInvoice/WorkingDocument Line: no TaxBase,
// no TaxPointDate, no References. Description follows UnitPrice directly.
type xmlMovementLine struct {
	LineNumber         int            `xml:"LineNumber"`
	OrderReferences    []xmlOrderRef  `xml:"OrderReferences,omitempty"`
	ProductCode        string         `xml:"ProductCode"`
	ProductDescription string         `xml:"ProductDescription"`
	Quantity           saftQty        `xml:"Quantity"`
	UnitOfMeasure      string         `xml:"UnitOfMeasure"`
	UnitPrice          saftMoneyLine  `xml:"UnitPrice"`
	Description        string         `xml:"Description"`
	DebitAmount        *saftMoneyLine `xml:"DebitAmount,omitempty"`
	CreditAmount       *saftMoneyLine `xml:"CreditAmount,omitempty"`
	Tax                *xmlTax        `xml:"Tax,omitempty"`
	TaxExemptionReason string         `xml:"TaxExemptionReason,omitempty"`
	TaxExemptionCode   string         `xml:"TaxExemptionCode,omitempty"`
	SettlementAmount   *saftMoneyLine `xml:"SettlementAmount,omitempty"`
}

type xmlMovementStatus struct {
	MovementStatus     string `xml:"MovementStatus"`
	MovementStatusDate string `xml:"MovementStatusDate"`
	Reason             string `xml:"Reason,omitempty"`
	SourceID           string `xml:"SourceID"`
	SourceBilling      string `xml:"SourceBilling"`
}

type xmlShippingPoint struct {
	DeliveryIDs  []string    `xml:"DeliveryID,omitempty"`
	DeliveryDate string      `xml:"DeliveryDate,omitempty"`
	WarehouseID  string      `xml:"WarehouseID,omitempty"`
	LocationID   string      `xml:"LocationID,omitempty"`
	Address      *xmlAddress `xml:"Address,omitempty"`
}

func buildMovementOfGoods(stock []domain.StockMovement, issuerEAC string) xmlMovementOfGoods {
	movements := make([]xmlStockMovement, 0, len(stock))
	var lineCount int
	var totalQty domain.Quantity
	for _, d := range stock {
		movements = append(movements, buildStockMovement(d, issuerEAC))
		// NumberOfMovementLines (4.2.1) counts every line, but
		// TotalQuantityIssued (4.2.2) excludes lines of MovementStatus "A"
		// documents (Portaria 302/2016).
		lineCount += len(d.Lines)
		if d.Status == domain.StatusCancelled {
			continue
		}
		for _, l := range d.Lines {
			totalQty += l.Quantity
		}
	}
	sortByKey(movements, func(m xmlStockMovement) string { return m.DocumentNumber })
	return xmlMovementOfGoods{
		NumberOfMovementLines: lineCount,
		TotalQuantityIssued:   saftQty(totalQty),
		Movements:             movements,
	}
}

func buildStockMovement(d domain.StockMovement, issuerEAC string) xmlStockMovement {
	lines := mapSlice(d.Lines, buildMovementLine)
	out := xmlStockMovement{
		DocumentNumber:    d.Number.Format(),
		ATCUD:             string(d.ATCUD),
		DocumentStatus:    buildMovementStatus(d.IssuedDocument),
		Hash:              string(d.Hash),
		HashControl:       string(d.HashControl),
		MovementDate:      fmtDate(d.Date),
		MovementType:      string(d.DocumentType),
		SystemEntryDate:   fmtDateTime(d.SystemEntryDate),
		CustomerID:        saftCustomerID(d.Customer.CustomerID),
		SourceID:          d.SourceID,
		EACCode:           issuerEAC,
		ShipTo:            buildShippingPoint(d.ShipTo),
		ShipFrom:          buildShippingPoint(d.ShipFrom),
		MovementStartTime: fmtDateTime(d.MovementStartTime),
		ATDocCodeID:       d.ATDocCodeID,
		Lines:             lines,
		DocumentTotals: xmlSimpleTotals{
			TaxPayable: saftMoney(d.Totals.TaxTotal + d.Totals.StampDuty),
			NetTotal:   saftMoney(d.Totals.NetTotal),
			GrossTotal: saftMoney(d.Totals.GrossTotal),
		},
	}
	if d.MovementEndTime != nil {
		out.MovementEndTime = fmtDateTime(*d.MovementEndTime)
	}
	return out
}

func buildMovementLine(l domain.DocumentLine) xmlMovementLine {
	net := saftMoneyLine(l.LineNetAmount())
	out := xmlMovementLine{
		LineNumber:         l.LineNumber,
		OrderReferences:    buildOrderRefs(l.OrderReferences),
		ProductCode:        l.Product.ProductCode,
		ProductDescription: l.Product.ProductDescription,
		Quantity:           saftQty(l.Quantity),
		UnitOfMeasure:      string(l.Product.Unit),
		UnitPrice:          saftMoneyLine(l.EffectiveUnitPrice()),
		Description:        l.Product.ProductDescription,
		CreditAmount:       &net,
	}
	if disc := l.LineDiscountAmount(); disc > 0 {
		v := saftMoneyLine(disc)
		out.SettlementAmount = &v
	}
	if l.Tax != nil {
		out.Tax, out.TaxExemptionReason, out.TaxExemptionCode = buildTax(l.Tax)
	}
	return out
}

func buildMovementStatus(d domain.IssuedDocument) xmlMovementStatus {
	return xmlMovementStatus{
		MovementStatus:     string(d.Status),
		MovementStatusDate: fmtDateTime(d.StatusDate),
		Reason:             d.Reason,
		SourceID:           d.SourceID,
		SourceBilling:      string(d.SourceBilling),
	}
}

func buildShippingPoint(sp *domain.ShippingPoint) *xmlShippingPoint {
	if sp == nil {
		return nil
	}
	out := &xmlShippingPoint{
		DeliveryIDs:  sp.DeliveryIDs,
		DeliveryDate: optDate(sp.DeliveryDate),
		WarehouseID:  sp.WarehouseID,
		LocationID:   sp.LocationID,
	}
	if sp.Address != nil {
		addr := buildAddress(*sp.Address)
		out.Address = &addr
	}
	return out
}
