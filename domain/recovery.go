package domain

import "time"

// IntegrateRecoveredDocument records a sales document that was emitted outside
// the normal certified path — either on paper during a system outage (manual)
// or via a backup certified system (Portaria 363/2010 Anexo II contingency).
// The resulting IssuedDocument carries SourceBilling = "M"; the monotonic-date
// and 5-working-day guards are bypassed, since recovery necessarily lands
// documents out of normal sequence (the originals may pre-date the prior
// recovered entry).
//
// draft.Date is the date printed on the paper or backup original. The two
// recovery sub-types (manual paper vs backup mirror) are distinguished at the
// signer / SourceID layer when production needs that audit-trail split — the
// domain treats them identically because both produce the same canonical state.
func IntegrateRecoveredDocument(draft *DraftSalesInvoice, series *Series, signer Signer, sourceID string, now time.Time) (SalesInvoice, error) {
	return IssueSalesInvoice(draft, series, signer, sourceID, now, IssueOptions{
		SourceBilling: SourceBillingManual,
		Recovery:      true,
	})
}
