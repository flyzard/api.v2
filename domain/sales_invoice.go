package domain

import (
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
}

// FSLimits caps the GrossTotal of an FS (simplified invoice / "fatura
// simplificada"). The retail tier applies when the issuer carries a retail
// EAC, the customer is the AnonymousCustomer pseudo-entity, and every line
// is goods (ProductType "P"). Any of those failing falls back to Default.
//
// Values are [CONFIRMAR] until legal confirms — DefaultFSLimits below carries
// the §0.5 fallback (€1000 retail / €100 default).
type FSLimits struct {
	Retail  Money
	Default Money
}

// DefaultFSLimits is the §0.5 fallback applied when IssueOptions.FSLimits is nil.
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

// SalesInvoice is the SAF-T SourceDocuments/SalesInvoices/Invoice for FT/FS/FR/NC/ND.
type SalesInvoice struct {
	IssuedDocument
	SalesInvoiceFields
}

// DraftSalesInvoice is the pre-issue sales invoice.
type DraftSalesInvoice struct {
	CommonDraftDocument
	SalesInvoiceFields
}

func (d *DraftSalesInvoice) Validate() error {
	if err := d.CommonDraftDocument.Validate(); err != nil {
		return err
	}
	if !d.DocumentType.IsSales() {
		return fmt.Errorf("not a sales doc type: %s", d.DocumentType)
	}
	// Sales lines require Tax per XSD; only Movement allows nil-Tax (non-valued GT, §5.11b).
	for i, line := range d.Lines {
		if line.Tax == nil {
			return fmt.Errorf("line %d: sales line requires Tax", i)
		}
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
	return nil
}

func IssueSalesInvoice(draft *DraftSalesInvoice, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions) (SalesInvoice, error) {
	if err := draft.Validate(); err != nil {
		return SalesInvoice{}, fmt.Errorf("draft: %w", err)
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
	issued, err := issueCommon(&draft.CommonDraftDocument, series, signer, sourceID, now, opts)
	if err != nil {
		return SalesInvoice{}, err
	}
	if draft.SpecialRegimes.SelfBilling {
		issued.Status = StatusSelfBilled
	}
	if len(draft.WithholdingTax) > 0 {
		issued.Totals.applyWithholding(draft.WithholdingTax)
	}
	return SalesInvoice{
		IssuedDocument:     issued,
		SalesInvoiceFields: draft.SalesInvoiceFields,
	}, nil
}
