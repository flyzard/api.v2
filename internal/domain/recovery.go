package domain

import (
	"fmt"
	"strings"
	"time"
)

// RecoveryKind distinguishes the two Portaria 363/2010 recovery flows. The
// byte value is the letter embedded in the M/D-form HashControl.
type RecoveryKind byte

const (
	RecoveryManual RecoveryKind = 'M' // paper doc (pre-printed) issued during an outage
	RecoveryBackup RecoveryKind = 'D' // doc issued by a backup certified system (Anexo II)
)

// RecoveredRef identifies the original document issued outside the certified
// path. It is encoded into the HashControl (Portaria 363/2010) — provenance
// lives there and in SourceBilling = "M"; no separate field is stored.
type RecoveredRef struct {
	Kind           RecoveryKind
	OriginalSeries string // série on the original, e.g. "F"
	OriginalNumber int    // sequence on the original, e.g. 23
	// OriginalType is the document type in the backup system. Required for
	// RecoveryBackup, forbidden for RecoveryManual. Not required to equal the
	// new document's type — a backup system may map types differently [CONFIRMAR].
	OriginalType DocumentType
}

func (r RecoveredRef) Validate() error {
	switch r.Kind {
	case RecoveryManual, RecoveryBackup:
	default:
		return fmt.Errorf("invalid recovery kind: %q", r.Kind)
	}
	if r.OriginalSeries == "" {
		return fmt.Errorf("original series is required")
	}
	// SAF-T XSD HashControl série token is [^/^ ]+ — no slash, caret, or space.
	if strings.ContainsAny(r.OriginalSeries, "/^ ") {
		return fmt.Errorf("original series cannot contain '/', '^' or space: %q", r.OriginalSeries)
	}
	if r.OriginalNumber < 1 {
		return fmt.Errorf("original number must be >= 1: %d", r.OriginalNumber)
	}
	if r.Kind == RecoveryBackup {
		if !r.OriginalType.IsValid() {
			return fmt.Errorf("backup recovery requires a valid original document type, got %q", r.OriginalType)
		}
	} else if r.OriginalType != "" {
		return fmt.Errorf("manual recovery cannot carry an original document type: %q", r.OriginalType)
	}
	return nil
}

// controlFor composes the Portaria 363/2010 M/D-form HashControl from the
// signer's key version, e.g. "1-FTM F/23" or "1-FTD FT D/3". The final shape
// is re-checked by HashControl.Validate at issuance.
func (r RecoveredRef) controlFor(keyVersion string, docType DocumentType) string {
	if r.Kind == RecoveryBackup {
		return fmt.Sprintf("%s-%s%c %s %s/%d", keyVersion, docType, r.Kind, r.OriginalType, r.OriginalSeries, r.OriginalNumber)
	}
	return fmt.Sprintf("%s-%s%c %s/%d", keyVersion, docType, r.Kind, r.OriginalSeries, r.OriginalNumber)
}

// IntegrateRecoveredStockMovement records a transport document emitted outside
// the certified path. The start-before-system-entry check (F-SAFT-16) is
// bypassed: a recovered paper guia necessarily started moving before its
// integration time.
func IntegrateRecoveredStockMovement(draft *DraftStockMovement, ref RecoveredRef, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions, qr QRConfig) (StockMovement, error) {
	opts.SourceBilling = SourceBillingManual
	opts.Recovered = &ref
	return IssueStockMovement(draft, series, signer, sourceID, now, opts, qr)
}

// IntegrateRecoveredWorkDocument records a working document emitted outside
// the certified path. The caller's opts pass through (same contract as
// IntegrateRecoveredSalesInvoice); SourceBilling and Recovered are forced here.
func IntegrateRecoveredWorkDocument(draft *DraftWorkDocument, ref RecoveredRef, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions, qr QRConfig) (WorkDocument, error) {
	opts.SourceBilling = SourceBillingManual
	opts.Recovered = &ref
	return IssueWorkDocument(draft, series, signer, sourceID, now, opts, qr)
}

// IntegrateRecoveredPayment records a receipt emitted outside the certified
// path. Receipts carry no Hash/HashControl in SAF-T, so there is no original
// reference to encode — recovery materializes as SourcePayment = "M" plus the
// recovery-series policy.
// No IssueOptions parameter: none of the issuance options apply to receipts
// (no signer, no calendar-gated guards), so there is nothing to pass through.
func IntegrateRecoveredPayment(draft *PaymentDraft, series *Series, now time.Time, totals PaymentTotals) (Payment, error) {
	return IssuePayment(draft, series, now, totals, IssueOptions{SourceBilling: SourceBillingManual})
}

// IntegrateRecoveredSalesInvoice records a sales document emitted outside the
// normal certified path — on paper during an outage (RecoveryManual) or via a
// backup certified system (RecoveryBackup, Portaria 363/2010 Anexo II).
//
// draft.Date is the date printed on the original. The caller's opts pass
// through so family invariants keep working (recovered FS needs
// opts.IssuerEAC/opts.FSLimits; recovered ND needs opts.Reader);
// SourceBilling and Recovered are forced here.
//
// The M16 gate is deliberately NOT bypassed: a recovered M16 document must
// still carry the EU non-PT customer with a VAT id — the original was only
// legal under those same RITI Art. 14.º conditions.
func IntegrateRecoveredSalesInvoice(draft *DraftSalesInvoice, ref RecoveredRef, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions, qr QRConfig) (SalesInvoice, error) {
	opts.SourceBilling = SourceBillingManual
	opts.Recovered = &ref
	return IssueSalesInvoice(draft, series, signer, sourceID, now, opts, qr)
}
