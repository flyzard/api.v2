package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// SalesInvoiceFields are the family-specific fields shared by DraftSalesInvoice (mutable,
// pre-issue) and SalesInvoice (immutable, post-issue). Frozen verbatim at issue time.
type SalesInvoiceFields struct {
	SpecialRegimes    SpecialRegimes   `json:"special_regimes"`
	ShipTo            *ShippingPoint   `json:"ship_to,omitempty"`
	ShipFrom          *ShippingPoint   `json:"ship_from,omitempty"`
	MovementStartTime *time.Time       `json:"movement_start_time,omitempty"`
	MovementEndTime   *time.Time       `json:"movement_end_time,omitempty"`
	WithholdingTax    []WithholdingTax `json:"withholding_tax,omitempty"`
	// Payments records FR (Fatura-Recibo) settlement entries. Required and
	// summing to GrossTotal only for DocumentType == FR (D-FR-1, D-FR-2;
	// F-SAFT-14 field shape). Multiple methods allowed; sum must match.
	Payments []FRPayment `json:"payments,omitempty"`
	// Currency is the SAF-T Invoice/DocumentTotals/Currency view for non-EUR
	// originals. Mirrors Payment.Currency. Date must equal the invoice Date.
	Currency *Currency `json:"currency,omitempty"`
	// GlobalDiscount is a document-level commercial discount (AT cert §5.7),
	// prorated into line nets at issue time (letter Nota 1: UnitPrice reflects
	// header discounts) and reported as DocumentTotals/Settlement/
	// SettlementAmount in SAF-T. Never carries line-discount sums (round 3348).
	GlobalDiscount Discount `json:"global_discount,omitempty"`
}

// FSLimits caps the GrossTotal of an FS (simplified invoice / "fatura
// simplificada"). The retail tier applies when the issuer carries a retail
// EAC, the customer is the AnonymousCustomer pseudo-entity, and every line
// is goods (ProductType "P"). Any of those failing falls back to Default.
//
// Values confirmed against CIVA Art. 40.º n.º 1: a) €1000 for retail goods to
// non-taxable persons, b) €100 otherwise; unchanged through 2026. Art. 53
// (isento) issuers may issue FS with no ceiling (n.º 1 c, DL 35/2025, in force
// 2025-07-01) — pass IssueOptions.FSLimits with unbounded values for those.
// Note Art. 40 carries its own exemption-motive element (n.º 2 e), so exempt
// lines are legal on an FS; no exempt-line restriction applies.
type FSLimits struct {
	Retail  Money
	Default Money
}

// DefaultFSLimits applies when IssueOptions.FSLimits is nil (Art. 40.º n.º 1 a/b).
var DefaultFSLimits = FSLimits{
	Retail:  Money(1000 * scale), // €1000
	Default: Money(100 * scale),  // €100
}

func (l FSLimits) resolveFor(draft *DraftSalesInvoice, issuerEAC string) Money {
	if !IsRetailActivity(issuerEAC) {
		return l.Default
	}
	if !draft.Customer.IsAnonymous() {
		return l.Default
	}
	for _, line := range draft.Lines {
		if line.Product.ProductType != ProductTypeGoods {
			return l.Default
		}
	}
	return l.Retail
}

// FRPayment is one SAF-T §4.1.4.20.6 settlement row attached to a Fatura-Recibo.
type FRPayment struct {
	Mechanism PaymentMechanism `json:"mechanism"`
	Amount    Money            `json:"amount"`
	Date      time.Time        `json:"date"`
}

// validateNDProductSet enforces F-SAFT-19: an ND can only adjust values, never
// introduce new products or change quantities. For each line, look up its
// References, find an originating line with the same ProductCode, and check
// that Quantity matches. Missing product = reject; mismatched quantity = reject.
func validateNDProductSet(draft *DraftSalesInvoice, reader IssuedDocumentReader) error {
	cache := make(map[string]IssuedDocument)
	for li, line := range draft.Lines {
		matched := false
		for _, ref := range line.References {
			num, err := ParseDocNumber(ref.Reference)
			if err != nil {
				continue
			}
			key := num.Format()
			orig, ok := cache[key]
			if !ok {
				orig, err = reader.FindByNumber(num)
				if err != nil {
					return fmt.Errorf("line %d: cannot resolve reference %q: %w", li, ref.Reference, err)
				}
				cache[key] = orig
			}
			for _, ol := range orig.Lines {
				if ol.Product.ProductCode != line.Product.ProductCode {
					continue
				}
				matched = true
				if ol.Quantity != line.Quantity {
					return fmt.Errorf("line %d: ND quantity %d differs from originating quantity %d for product %q (ND adjusts values, not quantities)",
						li, line.Quantity, ol.Quantity, line.Product.ProductCode)
				}
				break
			}
			if matched {
				break
			}
		}
		if !matched {
			return fmt.Errorf("line %d: product %q is not present on any referenced invoice (ND can only adjust pre-existing products)",
				li, line.Product.ProductCode)
		}
	}
	return nil
}

func (p FRPayment) Validate() error {
	if !p.Mechanism.IsValid() {
		return fmt.Errorf("invalid payment mechanism: %q", p.Mechanism)
	}
	if p.Amount <= 0 {
		return fmt.Errorf("payment amount must be positive: %d", p.Amount)
	}
	if p.Date.IsZero() {
		return fmt.Errorf("payment date is required")
	}
	return nil
}

// UnmarshalJSON peels off the polymorphic GlobalDiscount as a RawMessage and
// dispatches through unmarshalDiscount; everything else round-trips via the alias.
func (f *SalesInvoiceFields) UnmarshalJSON(data []byte) error {
	type alias SalesInvoiceFields
	aux := struct {
		*alias
		GlobalDiscount json.RawMessage `json:"global_discount,omitempty"`
	}{alias: (*alias)(f)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	d, err := unmarshalDiscount(aux.GlobalDiscount)
	if err != nil {
		return fmt.Errorf("global_discount: %w", err)
	}
	f.GlobalDiscount = d
	return nil
}

// SalesInvoice is the SAF-T SourceDocuments/SalesInvoices/Invoice for FT/FS/FR/NC/ND.
type SalesInvoice struct {
	IssuedDocument
	SalesInvoiceFields
}

// UnmarshalJSON decodes both embedded structs explicitly. Without this,
// SalesInvoiceFields.UnmarshalJSON would be promoted onto SalesInvoice and
// silently drop every IssuedDocument field.
func (s *SalesInvoice) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &s.IssuedDocument); err != nil {
		return err
	}
	return json.Unmarshal(data, &s.SalesInvoiceFields)
}

// DraftSalesInvoice is the pre-issue sales invoice.
type DraftSalesInvoice struct {
	CommonDraftDocument
	SalesInvoiceFields
}

// UnmarshalJSON — same promotion fix as SalesInvoice, for the draft side.
func (d *DraftSalesInvoice) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &d.CommonDraftDocument); err != nil {
		return err
	}
	return json.Unmarshal(data, &d.SalesInvoiceFields)
}

func (d *DraftSalesInvoice) Validate() error {
	if d.GlobalDiscount != nil {
		if err := d.GlobalDiscount.Validate(); err != nil {
			return fmt.Errorf("global discount: %w", err)
		}
	}
	if err := d.CommonDraftDocument.Validate(); err != nil {
		return err
	}
	if !d.DocumentType.IsSales() {
		return fmt.Errorf("not a sales doc type: %s", d.DocumentType)
	}
	if err := validateShipPoint("ship_to", d.ShipTo); err != nil {
		return err
	}
	if err := validateShipPoint("ship_from", d.ShipFrom); err != nil {
		return err
	}
	if d.MovementStartTime != nil && d.MovementEndTime != nil && d.MovementEndTime.Before(*d.MovementStartTime) {
		return fmt.Errorf("movement_end_time before movement_start_time")
	}
	for i, wh := range d.WithholdingTax {
		if err := wh.Validate(); err != nil {
			return fmt.Errorf("withholding_tax[%d]: %w", i, err)
		}
	}
	for i, p := range d.Payments {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("payments[%d]: %w", i, err)
		}
	}
	if d.DocumentType == FR && len(d.Payments) == 0 {
		return fmt.Errorf("FR requires at least one payment entry")
	}
	if d.GlobalDiscount != nil {
		_, sum := d.globalDiscountBases()
		if sum <= 0 {
			return ErrGlobalDiscountOnZeroNet
		}
		if a, ok := d.GlobalDiscount.(AmountDiscount); ok && a.Amount > sum {
			return ErrGlobalDiscountExceedsNet
		}
	}
	return nil
}

// globalDiscountBases is each line's post-line-discount net plus their sum —
// the proration weights of applyGlobalDiscount and the ceiling Validate holds
// the document-level discount against. One definition so the two can't drift.
func (d *DraftSalesInvoice) globalDiscountBases() ([]Money, Money) {
	bases := make([]Money, len(d.Lines))
	var sum Money
	for i, l := range d.Lines {
		bases[i] = applyDiscount(l.Discount, l.LineSubtotal())
		sum += bases[i]
	}
	return bases, sum
}

// applyGlobalDiscount prorates GlobalDiscount across the lines' post-line-
// discount nets at cent granularity (largest remainder, ties to the earlier
// line). It always recomputes from scratch: nil GlobalDiscount resets every
// share to zero, so clearing the field after a pre-issue CalculateTotals
// cannot leave orphaned shares baked into signed totals.
func (d *DraftSalesInvoice) applyGlobalDiscount() {
	for i := range d.Lines {
		d.Lines[i].GlobalDiscountShare = 0
	}
	if d.GlobalDiscount == nil {
		return
	}
	bases, sum := d.globalDiscountBases()
	if sum <= 0 {
		return // Validate rejects with ErrGlobalDiscountOnZeroNet
	}
	cents := roundDiv(int64(sum-applyDiscount(d.GlobalDiscount, sum)), centScale)
	if cents <= 0 {
		return
	}
	if cents*centScale > int64(sum) {
		cents = int64(sum) / centScale // near-100% discount on a sub-cent sum
	}
	shares := prorateCents(cents, bases)
	// prorateCents allocates whole cents; a largest-remainder bump can
	// overshoot a base with a sub-cent fraction (percent line discounts
	// produce those), which would sign a negative LineNetAmount. Clamp each
	// share to its line's whole-cent capacity and hand freed cents to lines
	// with slack; with no slack anywhere the realized discount shrinks by
	// the excess — SettlementAmount derives from Σ shares, so it stays
	// consistent either way.
	var excess int64
	for i, share := range shares {
		if lineCap := int64(bases[i]) / centScale * centScale; int64(share) > lineCap {
			excess += (int64(share) - lineCap) / centScale
			shares[i] = Money(lineCap)
		}
	}
	for excess > 0 {
		moved := false
		for i := range shares {
			if excess == 0 {
				break
			}
			if int64(bases[i])-int64(shares[i]) >= centScale {
				shares[i] += Money(centScale)
				excess--
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	for i, share := range shares {
		d.Lines[i].GlobalDiscountShare = share
	}
}

// CalculateTotals shadows the embedded method so every totals computation —
// pre-issue callers reading draft.Totals (currency blocks, FR payment sums)
// and issueCommon's recompute (dispatched here via totalsCalculator) — bakes
// the global-discount shares first.
func (d *DraftSalesInvoice) CalculateTotals() {
	d.applyGlobalDiscount()
	d.CommonDraftDocument.CalculateTotals()
}

func IssueSalesInvoice(draft *DraftSalesInvoice, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions, qr QRConfig) (SalesInvoice, error) {
	if err := draft.Validate(); err != nil {
		return SalesInvoice{}, fmt.Errorf("draft: %w", err)
	}
	// Mirrors IssuePayment: the FX rate must be dated on the invoice date so it
	// cannot drift between draft prep and issuance (SalesInvoiceFields.Currency contract).
	if draft.Currency != nil {
		date := draft.Date.In(lisbonLocation)
		// Normalize the rate date to Lisbon too: dateOnly keeps its operand's
		// location and time.Equal compares instants, so a UTC-located rate date
		// would otherwise fail on the same calendar day whenever Lisbon is on
		// summer time.
		if !dateOnly(draft.Currency.Date.In(lisbonLocation)).Equal(dateOnly(date)) {
			return SalesInvoice{}, fmt.Errorf("currency rate date %s does not match invoice date %s",
				draft.Currency.Date.Format("2006-01-02"), date.Format("2006-01-02"))
		}
		// Currency amount must equal the document gross total. Check before
		// issueCommon so a mismatch never advances the series counter (same
		// invariant as the FR and withholding guards below).
		draft.CalculateTotals()
		if draft.Currency.Amount != draft.Totals.GrossTotal {
			return SalesInvoice{}, fmt.Errorf("currency amount %s must equal document gross %s",
				draft.Currency.Amount.Format2DP(), draft.Totals.GrossTotal.Format2DP())
		}
	}
	if draft.DocumentType == ND {
		if opts.Reader == nil {
			return SalesInvoice{}, fmt.Errorf("ND requires IssueOptions.Reader to validate against originating invoice")
		}
		if err := validateNDProductSet(draft, opts.Reader); err != nil {
			return SalesInvoice{}, err
		}
	}
	if draft.DocumentType == FS {
		draft.CalculateTotals()
		limits := DefaultFSLimits
		if opts.FSLimits != nil {
			limits = *opts.FSLimits
		}
		threshold := limits.resolveFor(draft, opts.IssuerEAC)
		if draft.Totals.GrossTotal > threshold {
			return SalesInvoice{}, fmt.Errorf("FS gross %s exceeds limit %s", draft.Totals.GrossTotal.Format2DP(), threshold.Format2DP())
		}
	}
	// Withholding reduces AmountPayable (QR field P = O − payable); a sum
	// above the gross total would print a negative payable. Reject before
	// issueCommon so the series counter is untouched.
	if len(draft.WithholdingTax) > 0 {
		draft.CalculateTotals()
		var withheld Money
		for _, w := range draft.WithholdingTax {
			withheld += w.Amount
		}
		if withheld > draft.Totals.GrossTotal {
			return SalesInvoice{}, fmt.Errorf("withholding total %s exceeds gross total %s",
				withheld.Format2DP(), draft.Totals.GrossTotal.Format2DP())
		}
	}
	// FR sum check runs before issueCommon: issueCommon advances the series counter
	// (series.AppendIssue) on success, so a mismatch here must reject before that
	// mutation to avoid leaving a gap in the sequence.
	if draft.DocumentType == FR {
		draft.CalculateTotals()
		var sum Money
		for _, p := range draft.Payments {
			sum += p.Amount
		}
		if sum != draft.Totals.GrossTotal {
			return SalesInvoice{}, fmt.Errorf("FR payment sum %s does not match gross total %s",
				sum.Format2DP(), draft.Totals.GrossTotal.Format2DP())
		}
	}
	// draft (not the embedded struct) is the totalsCalculator: issueCommon's
	// totals recompute dispatches to the CalculateTotals override, which bakes
	// the global-discount shares before the canonical string is signed.
	issued, err := issueCommon(&draft.CommonDraftDocument, draft, series, signer, sourceID, now, opts)
	if err != nil {
		return SalesInvoice{}, err
	}
	if draft.SpecialRegimes.SelfBilling {
		issued.Status = StatusSelfBilled
	}
	if len(draft.WithholdingTax) > 0 {
		issued.Totals.applyWithholding(draft.WithholdingTax)
	}
	issued.QRPayload = buildQRPayload(&issued, qr)
	return SalesInvoice{
		IssuedDocument:     issued,
		SalesInvoiceFields: draft.SalesInvoiceFields.clone(),
	}, nil
}

func IsRetailActivity(eacCode string) bool {
	if len(eacCode) != 5 {
		return false
	}
	if eacCode[:2] == "47" {
		return true
	}
	// CAE-Rev.3 codes that count as motor-vehicle retail activity.
	switch eacCode {
	case "45110", "45190", "45320", "45401", "45402":
		return true
	}
	return false
}
