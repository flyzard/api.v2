package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// PaymentMechanism is the SAF-T PaymentMechanism enum (means of payment).
type PaymentMechanism string

const (
	PaymentMechanismCreditCard     PaymentMechanism = "CC"
	PaymentMechanismDebitCard      PaymentMechanism = "CD"
	PaymentMechanismCheck          PaymentMechanism = "CH"
	PaymentMechanismIntlCredit     PaymentMechanism = "CI"
	PaymentMechanismGiftCard       PaymentMechanism = "CO"
	PaymentMechanismBalanceComp    PaymentMechanism = "CS"
	PaymentMechanismElectronic     PaymentMechanism = "DE"
	PaymentMechanismCommercialBill PaymentMechanism = "LC"
	PaymentMechanismMultibanco     PaymentMechanism = "MB"
	PaymentMechanismCash           PaymentMechanism = "NU"
	PaymentMechanismOther          PaymentMechanism = "OU"
	PaymentMechanismBarter         PaymentMechanism = "PR"
	PaymentMechanismBankTransfer   PaymentMechanism = "TB"
	PaymentMechanismTitleVouchers  PaymentMechanism = "TR"

	MaxLenPaymentDescription = 200
)

func (m PaymentMechanism) IsValid() bool {
	switch m {
	case PaymentMechanismCreditCard, PaymentMechanismDebitCard, PaymentMechanismCheck,
		PaymentMechanismIntlCredit, PaymentMechanismGiftCard, PaymentMechanismBalanceComp,
		PaymentMechanismElectronic, PaymentMechanismCommercialBill, PaymentMechanismMultibanco,
		PaymentMechanismCash, PaymentMechanismOther, PaymentMechanismBarter,
		PaymentMechanismBankTransfer, PaymentMechanismTitleVouchers:
		return true
	}
	return false
}

// PaymentMethod is one means used to settle the receipt. Multiple methods may apply per receipt.
type PaymentMethod struct {
	Mechanism PaymentMechanism `json:"mechanism,omitempty"`
	Amount    Money            `json:"amount"`
	Date      time.Time        `json:"date"`
}

func (m PaymentMethod) Validate() error {
	if m.Mechanism != "" && !m.Mechanism.IsValid() {
		return fmt.Errorf("invalid payment mechanism: %s", m.Mechanism)
	}
	// Positive like FRPayment: a settlement row that moves no money is
	// meaningless. Mechanism stays optional here (XSD minOccurs=0 on receipts)
	// unlike FR rows, which must state how the invoice was paid.
	if m.Amount <= 0 {
		return fmt.Errorf("payment amount must be positive: %s", m.Amount)
	}
	if m.Date.IsZero() {
		return fmt.Errorf("payment date is required")
	}
	return nil
}

// SourceDocumentID references the originating invoice (or other source) that this payment settles.
type SourceDocumentID struct {
	OriginatingON string    `json:"originating_on"`
	InvoiceDate   time.Time `json:"invoice_date"`
	Description   string    `json:"description,omitempty"`
}

func (s SourceDocumentID) Validate() error {
	if s.OriginatingON == "" || len(s.OriginatingON) > MaxLenOriginatingON {
		return fmt.Errorf("originating_on length must be 1..%d", MaxLenOriginatingON)
	}
	if s.InvoiceDate.IsZero() {
		return fmt.Errorf("invoice_date is required")
	}
	if len(s.Description) > MaxLenPaymentDescription {
		return fmt.Errorf("description exceeds %d chars", MaxLenPaymentDescription)
	}
	return nil
}

// PaymentMovement is the sealed sum of debit/credit on a payment line. The SAF-T XSD
// uses a <choice> between DebitAmount and CreditAmount; modelling it as a nil-or-one
// interface removes the runtime mutual-exclusion check.
type PaymentMovement interface {
	Amount() Money
	isPaymentMovement()
}

// DebitAmount is the SAF-T DebitAmount variant: money leaving the customer ledger.
type DebitAmount struct {
	Value Money `json:"amount"`
}

func (d DebitAmount) Amount() Money    { return d.Value }
func (DebitAmount) isPaymentMovement() {}

// CreditAmount is the SAF-T CreditAmount variant: money credited to the customer ledger.
type CreditAmount struct {
	Value Money `json:"amount"`
}

func (c CreditAmount) Amount() Money    { return c.Value }
func (CreditAmount) isPaymentMovement() {}

// paymentMovementKind is the JSON discriminator for the PaymentMovement sealed sum.
type paymentMovementKind string

const (
	movementKindDebit  paymentMovementKind = "debit"
	movementKindCredit paymentMovementKind = "credit"
)

func (d DebitAmount) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   paymentMovementKind `json:"type"`
		Amount Money               `json:"amount"`
	}{movementKindDebit, d.Value})
}

func (c CreditAmount) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   paymentMovementKind `json:"type"`
		Amount Money               `json:"amount"`
	}{movementKindCredit, c.Value})
}

// unmarshalPaymentMovement picks the concrete variant from the type discriminator.
func unmarshalPaymentMovement(data []byte) (PaymentMovement, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var wire struct {
		Type   paymentMovementKind `json:"type"`
		Amount Money               `json:"amount"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	switch wire.Type {
	case "":
		return nil, nil
	case movementKindDebit:
		return DebitAmount{Value: wire.Amount}, nil
	case movementKindCredit:
		return CreditAmount{Value: wire.Amount}, nil
	default:
		return nil, fmt.Errorf("invalid movement type: %q", wire.Type)
	}
}

// PaymentLine is the SAF-T Payment/Line: a settlement entry against one or more source documents.
type PaymentLine struct {
	LineNumber       int                `json:"line_number"`
	SourceDocuments  []SourceDocumentID `json:"source_documents"`
	SettlementAmount *Money             `json:"settlement_amount,omitempty"`
	Movement         PaymentMovement    `json:"movement"`
	Tax              LineTax            `json:"tax,omitempty"`
}

// UnmarshalJSON peels off the polymorphic Movement and Tax fields as RawMessages
// and dispatches to the per-interface helpers; everything else round-trips through
// the alias.
func (l *PaymentLine) UnmarshalJSON(data []byte) error {
	type alias PaymentLine
	aux := struct {
		*alias
		Movement json.RawMessage `json:"movement"`
		Tax      json.RawMessage `json:"tax,omitempty"`
	}{alias: (*alias)(l)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m, err := unmarshalPaymentMovement(aux.Movement)
	if err != nil {
		return fmt.Errorf("movement: %w", err)
	}
	l.Movement = m
	tax, err := unmarshalLineTax(aux.Tax)
	if err != nil {
		return fmt.Errorf("tax: %w", err)
	}
	l.Tax = tax
	return nil
}

func (l PaymentLine) Validate() error {
	if l.LineNumber < 1 {
		return fmt.Errorf("line number must be >= 1, got %d", l.LineNumber)
	}
	if len(l.SourceDocuments) == 0 {
		return fmt.Errorf("at least one source_document required")
	}
	for i, sd := range l.SourceDocuments {
		if err := sd.Validate(); err != nil {
			return fmt.Errorf("source_documents[%d]: %w", i, err)
		}
	}
	if l.Movement == nil {
		return fmt.Errorf("PaymentLine requires a movement (debit or credit)")
	}
	if l.Movement.Amount() < 0 {
		return fmt.Errorf("negative movement amount")
	}
	if l.SettlementAmount != nil && *l.SettlementAmount < 0 {
		return fmt.Errorf("negative settlement_amount")
	}
	if l.Tax != nil {
		if err := l.Tax.Validate(); err != nil {
			return fmt.Errorf("tax: %w", err)
		}
	}
	return nil
}

// Payment is the SAF-T SourceDocuments/Payments/Payment for RC/RG receipts.
// Unlike IssuedDocument it carries no Hash/HashControl (per this XSD revision),
// but the series counter still advances at issue time.
type Payment struct {
	Number          DocNumber      `json:"number"`
	ATCUD           ATCUD          `json:"atcud"`
	TransactionID   string         `json:"transaction_id,omitempty"`
	TransactionDate time.Time      `json:"transaction_date"`
	Type            DocumentType   `json:"type"`
	Description     string         `json:"description,omitempty"`
	SystemID        string         `json:"system_id,omitempty"`
	Status          DocumentStatus `json:"status"`
	StatusDate      time.Time      `json:"status_date"`
	// Reason is the cancellation justification when Status == "A" (SAF-T PaymentStatus.Reason).
	Reason          string          `json:"reason,omitempty"`
	SourcePayment   SourcePayment   `json:"source_payment"`
	Methods         []PaymentMethod `json:"methods,omitempty"`
	SourceID        string          `json:"source_id"`
	SystemEntryDate time.Time       `json:"system_entry_date"`
	Customer        Customer        `json:"customer"`
	Lines           []PaymentLine   `json:"lines"`
	PaymentTotals
	Currency       *Currency        `json:"currency,omitempty"`
	WithholdingTax []WithholdingTax `json:"withholding_tax,omitempty"`
}

// PaymentDraft is the pre-issue payment carrying all the business data;
// IssuePayment turns it into a Payment with Number/ATCUD/SystemEntryDate set.
type PaymentDraft struct {
	Type            DocumentType
	TransactionDate time.Time
	TransactionID   string
	Description     string
	SystemID        string
	Customer        Customer
	SourceID        string
	Methods         []PaymentMethod
	Lines           []PaymentLine
	Currency        *Currency
	WithholdingTax  []WithholdingTax
}

func (d *PaymentDraft) Validate() error {
	if !d.Type.IsReceipt() {
		return fmt.Errorf("payment type must be a receipt doc type (RC or RG), got %s", d.Type)
	}
	if d.TransactionDate.IsZero() {
		return fmt.Errorf("transaction_date is required")
	}
	if d.Customer.CustomerID == uuid.Nil {
		return ErrMissingCustomer
	}
	if err := d.Customer.Validate(); err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	if d.SourceID == "" {
		return fmt.Errorf("source_id is required")
	}
	if len(d.Lines) == 0 {
		return fmt.Errorf("at least one line is required")
	}
	for i, m := range d.Methods {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("methods[%d]: %w", i, err)
		}
	}
	hasM16 := false
	// LineNumber collisions, like the sales family (CommonDraftDocument.Validate):
	// the projector copies LineNumber verbatim, so duplicates would reach the XML.
	seen := make(map[int]struct{}, len(d.Lines))
	for i, line := range d.Lines {
		if err := line.Validate(); err != nil {
			return fmt.Errorf("line %d: %w", i, err)
		}
		if _, dup := seen[line.LineNumber]; dup {
			return fmt.Errorf("line %d: duplicate LineNumber %d", i, line.LineNumber)
		}
		seen[line.LineNumber] = struct{}{}
		// XSD assert: Cash-VAT receipts (RC) require every line to carry Tax.
		if d.Type == RC && line.Tax == nil {
			return fmt.Errorf("line %d: RC requires Tax on every line", i)
		}
		hasM16 = hasM16 || lineExemption(line.Tax) == M16
	}
	if err := validateM16(d.Customer, hasM16); err != nil {
		return err
	}
	for i, wh := range d.WithholdingTax {
		if err := wh.Validate(); err != nil {
			return fmt.Errorf("withholding_tax[%d]: %w", i, err)
		}
	}
	return nil
}

// PaymentTotals mirrors SAF-T Payment.DocumentTotals. Supplied by the caller —
// the settlement line shape doesn't auto-compute the way product lines do.
type PaymentTotals struct {
	NetTotal   Money `json:"net_total"`
	TaxPayable Money `json:"tax_payable"`
	GrossTotal Money `json:"gross_total"`
}

// IssuePayment advances the series counter and produces an immutable Payment.
//
// CONCURRENCY: same rules as Issue — caller must serialize per Series.ID and run inside a transaction.
func IssuePayment(draft *PaymentDraft, series *Series, now time.Time, totals PaymentTotals, opts IssueOptions) (Payment, error) {
	if err := draft.Validate(); err != nil {
		return Payment{}, fmt.Errorf("draft: %w", err)
	}
	sourcePayment, err := opts.resolveSourceBilling()
	if err != nil {
		return Payment{}, err
	}
	if sourcePayment == SourceBillingIntegrated {
		return Payment{}, ErrIntegratedNotSupported
	}
	// Receipts carry no Hash/HashControl in SAF-T — there is nowhere to record
	// an original-document reference. Recovery for payments is SourcePayment="M"
	// plus the recovery-series policy only.
	if opts.Recovered != nil {
		return Payment{}, fmt.Errorf("IssueOptions.Recovered is not applicable to payments (receipts carry no HashControl)")
	}
	recovering := sourcePayment == SourceBillingManual
	txDate := draft.TransactionDate.In(lisbonLocation)
	sysEntry := now.In(lisbonLocation)
	// Rate date normalized to Lisbon like txDate — see the matching guard in
	// IssueSalesInvoice for why mixed locations break dateOnly comparison.
	if draft.Currency != nil && !dateOnly(draft.Currency.Date.In(lisbonLocation)).Equal(dateOnly(txDate)) {
		return Payment{}, fmt.Errorf("currency rate date %s does not match transaction date %s",
			draft.Currency.Date.Format("2006-01-02"), txDate.Format("2006-01-02"))
	}
	// PaymentDraft.Validate guarantees draft.Type is RC or RG; validateIssueContext
	// then enforces series.DocType == draft.Type, which implies series is a receipt.
	if err := validateIssueContext(series, draft.Type, draft.SourceID, txDate, sysEntry, recovering); err != nil {
		return Payment{}, err
	}
	if !recovering && series.LastDate != nil && dateOnly(txDate).Before(dateOnly(*series.LastDate)) {
		return Payment{}, fmt.Errorf("%w: %s < %s", ErrDateRegression, txDate.Format("2006-01-02"), series.LastDate.Format("2006-01-02"))
	}
	if totals.GrossTotal < 0 || totals.NetTotal < 0 || totals.TaxPayable < 0 {
		return Payment{}, fmt.Errorf("totals must be non-negative")
	}

	number, atcud, err := nextDocIdentity(series, draft.Type)
	if err != nil {
		return Payment{}, err
	}

	p := Payment{
		Number:          number,
		ATCUD:           atcud,
		TransactionID:   draft.TransactionID,
		TransactionDate: txDate,
		Type:            draft.Type,
		Description:     draft.Description,
		SystemID:        draft.SystemID,
		Status:          StatusNormal,
		StatusDate:      sysEntry,
		SourcePayment:   sourcePayment,
		Methods:         slices.Clone(draft.Methods),
		SourceID:        draft.SourceID,
		SystemEntryDate: sysEntry,
		Customer:        draft.Customer.clone(),
		Lines:           clonePaymentLines(draft.Lines),
		PaymentTotals:   totals,
		Currency:        clonePtr(draft.Currency),
		WithholdingTax:  slices.Clone(draft.WithholdingTax),
	}

	series.AppendIssue(number.Seq, "", txDate, sysEntry)

	return p, nil
}

// Cancel marks the payment as cancelled (Status = "A") if the e-Fatura
// deadline has not passed — same deadline rule as IssuedDocument.Cancel,
// anchored on TransactionDate. Receipts only transition N -> A
// (familyReceipt status set); there is no recovery flow past the deadline
// because receipts carry no HashControl.
func (p *Payment) Cancel(reason string, at time.Time) error {
	return applyCancel(&p.Status, &p.Reason, &p.StatusDate, p.TransactionDate, reason, at,
		StatusNormal)
}
