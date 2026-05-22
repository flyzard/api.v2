// Package-level seam: SAF-T (PT) projector contract.
//
// The SAF-T XML projector lives outside domain/ (Tier-3 module). Its expected
// input is the IssuedDocument value as produced by the family issuers in this
// package. This file documents the parts of that value the projector relies on
// so the contract is explicit and changes to domain types remain traceable to
// downstream impact.
//
// Required IssuedDocument fields:
//
//   - Number, ATCUD, Hash, HashControl, SystemEntryDate, SourceID,
//     SourceBilling, Period, Status, StatusDate, Reason, DocumentType,
//     Customer, Date, IssuedBy, Lines, Totals, PaymentTerms, QRPayload.
//
// Required Totals fields:
//
//   - NetTotal, TaxTotal, StampDuty, GrossTotal, AmountPayable,
//     Breakdown (sorted deterministically by (Region, Category, ExemptionCode)).
//
// Required per-line snapshots:
//
//   - Description (frozen at issue; equals Product.ProductDescription by
//     invariant — F-SAFT-9).
//   - LineNumber (auto-sequenced; never derived at projection).
//   - References / OrderReferences (verbatim).
//   - Tax (sealed sum: VATTax | StampTax | NotSubjectTax | nil-for-non-valued-guias).
//
// Required header fields supplied by the host application (NOT by domain):
//
//   - SoftwareIdentity: ProducerTaxID → Header.ProductCompanyTaxID;
//     ProductID() → Header.ProductID; CertificateNumber → Header.SoftwareCertificateNumber.
//   - Company (issuer) — separate from SoftwareIdentity per F-SAFT-4/5/6.
//
// Encoding: the projector MUST emit Windows-1252. All text-typed VO fields
// are pre-validated by enforceWindows1252 at construction (P1.5).
//
// Timezone: all time.Time fields are stored in Europe/Lisbon (P1.7). The
// projector formats them with the standard SAF-T layout
// ("2006-01-02" for dates, "2006-01-02T15:04:05" for system entry timestamps).
//
// Out of scope here: XSD validation, Win-1252 byte encoding, file packaging.

package domain
