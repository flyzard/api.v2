package domain

import (
	"fmt"
	"time"
)

// WorkDocument is the SAF-T SourceDocuments/WorkingDocuments/WorkDocument for OR/PF/NE/CM/FC/FO.
// Thinnest specialization — adds no fields beyond IssuedDocument.
type WorkDocument struct {
	IssuedDocument
}

// DraftWorkDocument is the pre-issue working document. Carries no family-specific fields;
// exists for symmetry with the other Draft* types and to anchor the IsWorking check pre-issue.
type DraftWorkDocument struct {
	CommonDraftDocument
}

func (d *DraftWorkDocument) Validate() error {
	if err := d.CommonDraftDocument.Validate(); err != nil {
		return err
	}
	if !d.DocumentType.IsWorking() {
		return fmt.Errorf("not a working doc type: %s", d.DocumentType)
	}
	return nil
}

// MarkBilled transitions a work document to Status = "F" (Faturado), capturing
// the reference to the sales invoice that consumed it. This is a one-way state
// change — re-marking errors. SAF-T M-2.
func (w *WorkDocument) MarkBilled(invoiceRef DocNumber, at time.Time) error {
	if err := invoiceRef.Validate(); err != nil {
		return fmt.Errorf("invoice ref: %w", err)
	}
	if w.Status == StatusBilled {
		return fmt.Errorf("work document already billed")
	}
	if w.Status != StatusNormal {
		return fmt.Errorf("cannot mark billed from status %q", w.Status)
	}
	w.Status = StatusBilled
	w.BilledByInvoice = &invoiceRef
	w.StatusDate = at.In(lisbonLocation)
	return nil
}

func IssueWorkDocument(draft *DraftWorkDocument, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions, qr QRConfig) (WorkDocument, error) {
	if err := draft.Validate(); err != nil {
		return WorkDocument{}, fmt.Errorf("draft: %w", err)
	}
	issued, err := issueCommon(&draft.CommonDraftDocument, &draft.CommonDraftDocument, series, signer, sourceID, now, opts)
	if err != nil {
		return WorkDocument{}, err
	}
	issued.QRPayload = buildQRPayload(&issued, qr)
	return WorkDocument{IssuedDocument: issued}, nil
}
