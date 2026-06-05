package domain

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

type Totals struct {
	NetTotal   Money `json:"net_total"`
	TaxTotal   Money `json:"tax_total"`
	StampDuty  Money `json:"stamp_duty"`
	GrossTotal Money `json:"gross_total"`
	// AmountPayable defaults to GrossTotal. Family issuers that carry
	// WithholdingTax (sales / payment) subtract the total withheld here so the
	// final figure is "amount the customer actually pays" — feeds QR field P
	// and the SAF-T DocumentTotals.GrossTotal vs payable distinction.
	AmountPayable Money        `json:"amount_payable"`
	Breakdown     TaxBreakdown `json:"breakdown,omitempty"`
}

// DocumentCore is the set of fields shared verbatim by CommonDraftDocument
// (pre-issue) and IssuedDocument (post-issue). Adding a shared field here
// guarantees both sides stay in sync without duplicating JSON tags.
type DocumentCore struct {
	DocumentType DocumentType   `json:"doc_type"`
	Customer     Customer       `json:"customer"`
	Date         time.Time      `json:"date"`
	IssuedBy     User           `json:"issued_by"`
	Lines        []DocumentLine `json:"lines"`
	Totals       Totals         `json:"totals,omitzero"`
	PaymentTerms *time.Time     `json:"payment_terms,omitempty"`
}

type CommonDraftDocument struct {
	DocumentCore
	Series Series `json:"series"`
}

// AddLine appends a line, assigning LineNumber from the current length so callers
// cannot create gaps or collisions. Any LineNumber set by the caller is overwritten.
func (d *CommonDraftDocument) AddLine(line DocumentLine) {
	line.LineNumber = len(d.Lines) + 1
	d.Lines = append(d.Lines, line)
}

func (d *CommonDraftDocument) Validate() error {
	if d.DocumentType == "" {
		return ErrMissingDocumentType
	}
	rules, ok := documentTypes[d.DocumentType]
	if !ok {
		return fmt.Errorf("%w: %q", ErrInvalidDocumentType, d.DocumentType)
	}
	if d.Customer.CustomerID == uuid.Nil {
		return ErrMissingCustomer
	}
	if d.Series.ID == "" {
		return ErrMissingSeries
	}
	if d.Date.IsZero() {
		return ErrMissingDate
	}
	if d.PaymentTerms != nil && d.PaymentTerms.Before(d.Date) {
		return fmt.Errorf("payment_terms %s precedes document date %s", d.PaymentTerms.Format("2006-01-02"), d.Date.Format("2006-01-02"))
	}
	if len(d.Lines) == 0 {
		return ErrNoLines
	}
	seen := make(map[int]struct{}, len(d.Lines))
	hasM16 := false
	for i, line := range d.Lines {
		if err := line.Validate(); err != nil {
			return fmt.Errorf("line %d: %w", i, err)
		}
		if _, dup := seen[line.LineNumber]; dup {
			return fmt.Errorf("line %d: duplicate LineNumber %d", i, line.LineNumber)
		}
		seen[line.LineNumber] = struct{}{}
		if rules.RequiresRef && len(line.References) == 0 {
			return fmt.Errorf("line %d: %s requires References (AT business rule)", i, d.DocumentType)
		}
		// Sales/working lines require Tax per XSD; only Movement allows nil-Tax (non-valued GT, §5.11b).
		if rules.RequiresLineTax && line.Tax == nil {
			return fmt.Errorf("line %d: %s requires Tax on every line", i, d.DocumentType)
		}
		if !rules.AllowsStamp {
			if _, isStamp := line.Tax.(StampTax); isStamp {
				return fmt.Errorf("line %d: stamp duty not allowed on %s", i, d.DocumentType)
			}
		}
		hasM16 = hasM16 || lineExemption(line.Tax) == M16
	}
	return validateM16(d.Customer, hasM16)
}

// validateM16 enforces the substantive conditions of RITI Art. 14.º n.º 1 a)
// — per Ofício-Circulado 30225/2020 the buyer's VAT registration in another
// Member State is a substantive (not formal) condition of the intra-community
// exemption — to the extent checkable at issuance: buyer in another EU member
// state with a real VAT identification. It applies to every document family:
// intra-EU transfer guias and receipts carry M16 lines too, not only invoices.
//
// Known limits, deliberate: an empty BillingAddress.Country is rejected
// (conservative — the substantive condition can't be confirmed without it);
// VAT-id/country consistency is NOT checked because SAF-T stores the tax id
// without its country prefix. Transport evidence and VIES liveness are outside
// software reach at issue time and stay the issuer's burden.
func validateM16(customer Customer, hasM16 bool) error {
	if !hasM16 {
		return nil
	}
	country := customer.BillingAddress.Country
	if country == "PT" || !euMemberStates[country] {
		return fmt.Errorf("M16 (Art. 14.º RITI) requires a customer in another EU member state, got country %q", country)
	}
	if id := customer.CustomerTaxID; id == "" || id == FinalConsumerNIF {
		return fmt.Errorf("M16 (Art. 14.º RITI) requires the customer's VAT identification number")
	}
	return nil
}

// CalculateTotals folds line subtotals into d.Totals.
// VAT and Stamp Duty are tracked separately because SAF-T exports them as distinct totals;
// TaxPayable (TaxTotal + StampDuty) is reassembled at export time.
//
// The VAT breakdown is also accumulated per (Region, Category, ExemptionCode) so the
// SAF-T DocumentTotals projection and the QR I/J/K series can be derived without
// re-walking lines. TODO(NS-breakdown): NotSubjectTax lines contribute to NetTotal
// but are not yet aggregated into the breakdown; add when the SAF-T projector lands.
func (d *CommonDraftDocument) CalculateTotals() {
	var t Totals
	bd := make(map[taxBreakdownKey]TaxBreakdownEntry)
	for _, line := range d.Lines {
		afterDiscount := line.LineNetAmount()
		// regras R-L: discount is applied to LineSubtotal, never to TaxBase.
		// When TaxBase is set the line is a tax-only adjustment — UnitPrice is 0
		// (enforced by DocumentLine.Validate) so afterDiscount is 0 anyway.
		taxBase := afterDiscount
		if line.TaxBase != nil && *line.TaxBase != 0 {
			taxBase = *line.TaxBase
		}
		t.NetTotal += afterDiscount
		if line.Tax == nil {
			t.GrossTotal += afterDiscount
			continue
		}
		taxAmount := line.Tax.Apply(taxBase)
		switch tax := line.Tax.(type) {
		case VATTax:
			t.TaxTotal += taxAmount
			key := taxBreakdownKey{tax.Rate.Region, tax.Rate.Category, tax.Rate.Exemption}
			entry, exists := bd[key]
			if !exists {
				entry = TaxBreakdownEntry{
					Region:               tax.Rate.Region,
					Category:             tax.Rate.Category,
					ExemptionCode:        tax.Rate.Exemption,
					ExemptionDescription: tax.Rate.Exemption.Description(),
				}
			}
			entry.Base += taxBase
			entry.Tax += taxAmount
			bd[key] = entry
		case StampTax:
			t.StampDuty += taxAmount
		}
		t.GrossTotal += afterDiscount + taxAmount
	}
	t.Breakdown = sortTaxBreakdown(bd)
	t.AmountPayable = t.GrossTotal
	d.Totals = t
}

// applyWithholding subtracts Σ wht.Amount from t.AmountPayable. Used by family
// issuers (sales / payment) that hold WithholdingTax outside CommonDraftDocument.
func (t *Totals) applyWithholding(wht []WithholdingTax) {
	var sum Money
	for _, w := range wht {
		sum += w.Amount
	}
	t.AmountPayable = t.GrossTotal - sum
}

func sortTaxBreakdown(m map[taxBreakdownKey]TaxBreakdownEntry) TaxBreakdown {
	out := make(TaxBreakdown, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b TaxBreakdownEntry) int {
		return cmp.Or(
			cmp.Compare(a.Region, b.Region),
			cmp.Compare(a.Category, b.Category),
			cmp.Compare(a.ExemptionCode, b.ExemptionCode),
		)
	})
	return out
}
