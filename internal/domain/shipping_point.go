package domain

import (
	"fmt"
	"time"
)

const (
	MaxLenWarehouseID = 50
	MaxLenLocationID  = 30
	MaxLenDeliveryID  = 200
)

// ShippingPoint is the SAF-T ShippingPointStructure used by ShipTo / ShipFrom on
// SalesInvoice (transport-relevant invoices) and StockMovement documents.
type ShippingPoint struct {
	DeliveryIDs  []string   `json:"delivery_ids,omitempty"`
	DeliveryDate *time.Time `json:"delivery_date,omitempty"`
	WarehouseID  string     `json:"warehouse_id,omitempty"`
	LocationID   string     `json:"location_id,omitempty"`
	Address      *Address   `json:"address,omitempty"`
}

// validateShipPoint runs Validate on an optional ShippingPoint and wraps the error
// with a descriptive label. Used by document families that carry ShipTo / ShipFrom.
func validateShipPoint(label string, sp *ShippingPoint) error {
	if sp == nil {
		return nil
	}
	if err := sp.Validate(); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func (sp ShippingPoint) Validate() error {
	for i, id := range sp.DeliveryIDs {
		if id == "" || len(id) > MaxLenDeliveryID {
			return fmt.Errorf("delivery_id[%d] length must be 1..%d", i, MaxLenDeliveryID)
		}
	}
	if len(sp.WarehouseID) > MaxLenWarehouseID {
		return fmt.Errorf("warehouse_id exceeds %d chars", MaxLenWarehouseID)
	}
	if len(sp.LocationID) > MaxLenLocationID {
		return fmt.Errorf("location_id exceeds %d chars", MaxLenLocationID)
	}
	if sp.Address != nil {
		if err := sp.Address.Validate(); err != nil {
			return fmt.Errorf("address: %w", err)
		}
	}
	return nil
}
