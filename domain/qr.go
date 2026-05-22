// Package-level seam: QR-code generator contract.
//
// The QR generator lives outside domain/ (Tier-3 module). Its output is
// written verbatim into IssuedDocument.QRPayload at issuance and is reprinted
// unchanged on every copy (F-QR-3 — payload frozen at original issuance state,
// not at reprint time).
//
// Field source mapping (Portaria 195/2020 + AT FAQ 4443):
//
//   A  Issuer NIF                  Company.NIF (host-supplied)
//   B  Customer NIF                IssuedDocument.Customer.CustomerTaxID
//   C  Customer country            IssuedDocument.Customer.BillingAddress.Country (or "Desconhecido")
//   D  DocumentType                IssuedDocument.DocumentType
//   E  Status (frozen)             IssuedDocument.Status at issuance — NOT current status
//   F  Date                        IssuedDocument.Date ("2006-01-02", Europe/Lisbon)
//   G  Document number             IssuedDocument.Number.Format()
//   H  ATCUD                       IssuedDocument.ATCUD
//   I1..I8  PT mainland VAT block  Totals.Breakdown entries where Region == PT
//   J1..J8  Açores VAT block        Totals.Breakdown entries where Region == PT-AC
//   K1..K8  Madeira VAT block       Totals.Breakdown entries where Region == PT-MA
//   L  Non-VAT taxes               Totals.StampDuty + similar non-VAT lines
//   M  Stamp duty total            Totals.StampDuty
//   N  Tax total                   Totals.TaxTotal
//   O  Gross total                 Totals.GrossTotal
//   P  Withholding total           GrossTotal − AmountPayable (i.e. Σ WithholdingTax.Amount)
//   Q  Hash (4 chars)              IssuedDocument.Hash, first 4 chars
//   R  Certificate number          SoftwareIdentity.CertificateNumber (host-supplied)
//   S  Other info (frozen)         Application-defined; frozen with the rest of the payload
//
// QR encoding requirements (FAQ 4443):
//   - QR version >= 9
//   - Error-correction level M
//   - Field separator "*", key/value separator ":"
//   - Decimal separator "." (Portuguese SAF-T 2-decimal convention)
//
// Cancellation invariance: QRPayload is captured at issuance and MUST NOT be
// recomputed when Status flips to "A" (Cancel) or "F" (MarkBilled). The
// reprint shows the original-issuance QR even after cancellation; the new
// status is conveyed via separate print markings, not via QR mutation.

package domain
