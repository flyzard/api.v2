// Package-level seam: QR-code contract.
//
// The QR PAYLOAD STRING is composed in domain by buildQRPayload (qr_builder.go) —
// pure, deterministic, no infra — and assigned to IssuedDocument.QRPayload at
// issuance. Only the IMAGE rasterization (QR version >= 9, ECC M, PNG/SVG) is a
// Tier-3 concern, consuming the frozen string at print time. The payload is
// reprinted unchanged on every copy (F-QR-3 — frozen at original issuance state,
// not at reprint time).
//
// Field source mapping (Portaria 195/2020 + AT FAQ 4443):
//
//	A  Issuer NIF                  Company.NIF (host-supplied)
//	B  Customer NIF                IssuedDocument.Customer.CustomerTaxID
//	C  Customer country            IssuedDocument.Customer.BillingAddress.Country (or "Desconhecido")
//	D  DocumentType                IssuedDocument.DocumentType
//	E  Status (frozen)             IssuedDocument.Status at issuance — NOT current status
//	F  Date                        IssuedDocument.Date ("20060102" compact, Europe/Lisbon)
//	G  Document number             IssuedDocument.Number.Format()
//	H  ATCUD                       IssuedDocument.ATCUD
//	I1..I8  PT mainland VAT block  Totals.Breakdown entries where Region == PT
//	J1..J8  Açores VAT block        Totals.Breakdown entries where Region == PT-AC
//	K1..K8  Madeira VAT block       Totals.Breakdown entries where Region == PT-MA
//	L  Non-VAT base                Total amount not subject to / not taxable under VAT
//	M  Stamp duty total            Totals.StampDuty
//	N  Tax total (TaxPayable)      Totals.TaxTotal + Totals.StampDuty
//	O  Gross total                 Totals.GrossTotal
//	P  Withholding total           GrossTotal − AmountPayable (i.e. Σ WithholdingTax.Amount)
//	Q  Hash (4 chars)              IssuedDocument.Hash chars at positions 1, 11, 21, 31
//	R  Certificate number          SoftwareIdentity.CertificateNumber (host-supplied)
//	S  Other info (frozen)         Application-defined; frozen with the rest of the payload
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

import "strings"

// QRConfig carries the three QR values that are not part of the domain document:
// issuer NIF (field A), software certificate number (field R), and the frozen
// app-defined "other info" (field S). Sourced from Company.NIF and
// config.SoftwareIdentity.CertificateNumber at the call site.
type QRConfig struct {
	IssuerNIF         TaxID
	CertificateNumber string
	OtherInfo         string
}

// buildQRPayload assembles the Portaria 195/2020 QR string from an already-issued
// document plus the host-supplied QRConfig. Pure and deterministic: same inputs
// always yield the same string. The result is stored verbatim in
// IssuedDocument.QRPayload at issuance and never recomputed (see package comment).
//
// Field semantics and ordering are documented in the package comment above. Money is rendered
// 2-decimal; the field separator is "*" and the key/value separator is ":".
func buildQRPayload(d *IssuedDocument, cfg QRConfig) string {
	var parts []string
	add := func(key, val string) { parts = append(parts, key+":"+val) }

	add("A", string(cfg.IssuerNIF))
	add("B", string(d.Customer.CustomerTaxID))
	country := string(d.Customer.BillingAddress.Country)
	if country == "" {
		country = "Desconhecido"
	}
	add("C", country)
	add("D", string(d.DocumentType))
	add("E", string(d.Status))
	// Date is already Lisbon-normalized at issuance (issueCommon); format it
	// directly, matching the SAF-T projector convention (saft fmtDate).
	add("F", d.Date.Format("20060102"))
	add("G", d.Number.Format())
	add("H", string(d.ATCUD))

	rollups := rollupByRegion(d.Totals.Breakdown)
	before := len(parts)
	appendRegionBlock(&parts, "I", PT, rollups[PT])
	appendRegionBlock(&parts, "J", PTAC, rollups[PTAC])
	appendRegionBlock(&parts, "K", PTMA, rollups[PTMA])
	if len(parts) == before {
		// Spec rules (g)/(h): at least one fiscal space must always exist; a
		// document with no VAT-rate indication (e.g. non-valued guia) uses the
		// region-code placeholder "0".
		parts = append(parts, "I1:0")
	}

	if l := nonSubjectBase(d); l != 0 {
		add("L", l.Format2DP())
	}
	if d.Totals.StampDuty != 0 {
		add("M", d.Totals.StampDuty.Format2DP())
	}
	// N is TaxPayable = total VAT + stamp duty (Portaria 195/2020 field N), not
	// VAT alone — TaxTotal and StampDuty are kept separate in Totals.
	add("N", (d.Totals.TaxTotal + d.Totals.StampDuty).Format2DP())
	add("O", d.Totals.GrossTotal.Format2DP())
	if p := d.Totals.GrossTotal - d.Totals.AmountPayable; p != 0 {
		add("P", p.Format2DP())
	}
	add("Q", d.Hash.FourChars())
	add("R", cfg.CertificateNumber)
	if s := sanitizeS(cfg.OtherInfo); s != "" {
		add("S", s)
	}

	return strings.Join(parts, "*")
}

// regionRollup holds the per-region VAT sums the QR I/J/K blocks need, bucketed by
// rate category.
type regionRollup struct {
	baseISE, baseRED, taxRED, baseINT, taxINT, baseNOR, taxNOR Money
}

// rollupByRegion buckets the breakdown's Base/Tax by region and rate category.
// TaxOther/OUT has no canonical QR sub-field (Portaria 195/2020 I-block covers
// only ISE/RED/INT/NOR) and is intentionally skipped.
func rollupByRegion(bd TaxBreakdown) map[TaxRegion]*regionRollup {
	out := make(map[TaxRegion]*regionRollup)
	for _, e := range bd {
		r := out[e.Region]
		if r == nil {
			r = &regionRollup{}
			out[e.Region] = r
		}
		switch e.Category {
		case TaxExempt:
			r.baseISE += e.Base
		case TaxReduced:
			r.baseRED += e.Base
			r.taxRED += e.Tax
		case TaxIntermediate:
			r.baseINT += e.Base
			r.taxINT += e.Tax
		case TaxNormal:
			r.baseNOR += e.Base
			r.taxNOR += e.Tax
		}
	}
	return out
}

// appendRegionBlock emits x1 (region code, always) then x2..x8 omit-if-zero.
// A region present in the breakdown always emits at least its region code (x1),
// even when every rate bucket is zero (e.g. OUT-only lines, which have no QR
// sub-field but still occupy a fiscal space). A region absent from the breakdown
// produces nothing.
func appendRegionBlock(parts *[]string, prefix string, code TaxRegion, r *regionRollup) {
	if r == nil {
		return
	}
	*parts = append(*parts, prefix+"1:"+string(code))
	addNZ(parts, prefix+"2", r.baseISE)
	addNZ(parts, prefix+"3", r.baseRED)
	addNZ(parts, prefix+"4", r.taxRED)
	addNZ(parts, prefix+"5", r.baseINT)
	addNZ(parts, prefix+"6", r.taxINT)
	addNZ(parts, prefix+"7", r.baseNOR)
	addNZ(parts, prefix+"8", r.taxNOR)
}

// addNZ appends "<key>:<2dp>" only when v is non-zero.
func addNZ(parts *[]string, key string, v Money) {
	if v != 0 {
		*parts = append(*parts, key+":"+v.Format2DP())
	}
}

// nonSubjectBase computes field L: the total base of lines outside the scope of
// VAT (NotSubjectTax). Such lines carry a non-nil Tax and produce no Breakdown
// entry, so the line walk is required. Uses LineNetAmount (post-discount,
// pre-tax) so L matches the share these lines contribute to NetTotal.
//
// L is sourced from Lines while the I/J/K blocks come from Totals.Breakdown. If
// CalculateTotals's TODO(NS-breakdown) (document.go) ever makes NotSubjectTax
// lines produce Breakdown entries, this walk must be reconciled so L is not
// double-counted.
func nonSubjectBase(d *IssuedDocument) Money {
	var sum Money
	for _, line := range d.Lines {
		if _, ns := line.Tax.(NotSubjectTax); ns {
			sum += line.LineNetAmount()
		}
	}
	return sum
}

// sanitizeS strips the asterisk from the free-text S field — Portaria 195/2020
// forbids only "*" there (it is the field separator). ":" is permitted in S.
func sanitizeS(s string) string {
	return strings.ReplaceAll(s, "*", "")
}
