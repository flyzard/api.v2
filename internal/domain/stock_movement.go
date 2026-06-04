package domain

import (
	"fmt"
	"time"
)

// StockMovementFields are the family-specific fields shared by DraftStockMovement (mutable,
// pre-issue) and StockMovement (immutable, post-issue). Frozen verbatim at issue time.
// MovementStartTime is mandatory per XSD.
type StockMovementFields struct {
	MovementStartTime time.Time      `json:"movement_start_time"`
	MovementEndTime   *time.Time     `json:"movement_end_time,omitempty"`
	ATDocCodeID       string         `json:"at_doc_code_id,omitempty"`
	ShipFrom          *ShippingPoint `json:"ship_from,omitempty"`
	ShipTo            *ShippingPoint `json:"ship_to,omitempty"`
	// ThirdParties marks "Por conta de terceiros" guias. Drives DocumentStatus = "T"
	// per AT public-docs (§0.5 fallback in FIX_PLAN.md until SAF-T XSD is obtained).
	ThirdParties bool `json:"third_parties,omitempty"`
}

// StockMovement is the SAF-T SourceDocuments/MovementOfGoods/StockMovement for GT/GR/GA/GC/GD.
// Lines must carry MovementTax (IVA or NS only) — stamp duty is rejected.
type StockMovement struct {
	IssuedDocument
	StockMovementFields
}

// DraftStockMovement is the pre-issue stock movement.
type DraftStockMovement struct {
	CommonDraftDocument
	StockMovementFields
}

func (d *DraftStockMovement) Validate() error {
	if err := d.CommonDraftDocument.Validate(); err != nil {
		return err
	}
	if !d.DocumentType.IsTransport() {
		return fmt.Errorf("not a transport doc type: %s", d.DocumentType)
	}
	// Valued vs non-valued path (F-NEW-8 / F-SAFT-18): a guia with ANY priced line
	// must price every line; an all-zero-UnitPrice guia may omit Tax entirely.
	valued := false
	for _, line := range d.Lines {
		if line.UnitPrice > 0 {
			valued = true
			break
		}
	}
	for i, line := range d.Lines {
		if valued && line.Tax == nil {
			return fmt.Errorf("line %d: valued guia requires tax on every line", i)
		}
	}
	if d.MovementStartTime.IsZero() {
		return fmt.Errorf("movement_start_time is required")
	}
	if d.MovementEndTime != nil && d.MovementEndTime.Before(d.MovementStartTime) {
		return fmt.Errorf("movement_end_time before movement_start_time")
	}
	if len(d.ATDocCodeID) > 200 {
		return fmt.Errorf("at_doc_code_id exceeds 200 chars")
	}
	if d.ShipFrom == nil {
		return fmt.Errorf("ship_from is required on transport documents")
	}
	if d.ShipTo == nil {
		return fmt.Errorf("ship_to is required on transport documents")
	}
	if err := validateShipPoint("ship_from", d.ShipFrom); err != nil {
		return err
	}
	if err := validateShipPoint("ship_to", d.ShipTo); err != nil {
		return err
	}
	return nil
}

func IssueStockMovement(draft *DraftStockMovement, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions, qr QRConfig) (StockMovement, error) {
	if err := draft.Validate(); err != nil {
		return StockMovement{}, fmt.Errorf("draft: %w", err)
	}
	issued, err := issueCommon(&draft.CommonDraftDocument, series, signer, sourceID, now, opts)
	if err != nil {
		return StockMovement{}, err
	}
	// F-SAFT-16: a guia cannot be issued AFTER the goods have already moved
	// (no retro-active transport). Compare on the same Lisbon clock the system
	// entry was stamped with. Skipped under recovery — a recovered paper guia
	// necessarily started moving before its integration time.
	if opts.Recovered == nil {
		startLisbon := draft.MovementStartTime.In(lisbonLocation)
		if startLisbon.Before(issued.SystemEntryDate) {
			return StockMovement{}, fmt.Errorf("movement_start_time %s precedes system entry %s",
				startLisbon.Format(time.RFC3339), issued.SystemEntryDate.Format(time.RFC3339))
		}
	}
	if draft.ThirdParties {
		issued.Status = StatusThirdParty
	}
	issued.QRPayload = buildQRPayload(&issued, qr)
	return StockMovement{
		IssuedDocument:      issued,
		StockMovementFields: draft.StockMovementFields,
	}, nil
}
