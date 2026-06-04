package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	_ "time/tzdata"
)

// DocumentStatus is the SAF-T DocumentStatus value, shared by all four document families.
type DocumentStatus string

const MaxLenCancellationReason = 100

// lisbonLocation is the canonical clock for AT certification.
var lisbonLocation = mustLisbonLocation()

func mustLisbonLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("domain: cannot load Europe/Lisbon timezone (tzdata missing?): " + err.Error())
	}
	return loc
}

const (
	StatusNormal     DocumentStatus = "N" // all families
	StatusSelfBilled DocumentStatus = "S" // sales invoices only
	StatusCancelled  DocumentStatus = "A" // all families
	StatusBilled     DocumentStatus = "F" // sales / movement / work
	StatusSummary    DocumentStatus = "R" // sales / movement
	StatusThirdParty DocumentStatus = "T" // stock movements only
)

// allowedStatuses lists the DocumentStatus codes legal per SAF-T document family.
// Keyed by family so adding a new doctype in a known family is a no-op.
var allowedStatuses = map[docFamily][]DocumentStatus{
	familySales:     {StatusNormal, StatusSelfBilled, StatusCancelled, StatusSummary, StatusBilled},
	familyTransport: {StatusNormal, StatusThirdParty, StatusCancelled, StatusBilled, StatusSummary},
	familyWorking:   {StatusNormal, StatusCancelled, StatusBilled},
	familyReceipt:   {StatusNormal, StatusCancelled},
}

func (s DocumentStatus) ValidFor(dt DocumentType) bool {
	rules, ok := documentTypes[dt]
	if !ok {
		return false
	}
	return slices.Contains(allowedStatuses[rules.Family], s)
}

// SourceBilling marks whether a document was produced in this application (P),
// integrated from another app (I), or recovered/emitted manually (M).
// SAF-T uses the identical enum as SourcePayment for receipts.
type SourceBilling string

const (
	SourceBillingProduced   SourceBilling = "P"
	SourceBillingIntegrated SourceBilling = "I"
	SourceBillingManual     SourceBilling = "M"
)

func (b SourceBilling) IsValid() bool {
	switch b {
	case SourceBillingProduced, SourceBillingIntegrated, SourceBillingManual:
		return true
	}
	return false
}

// SourcePayment is the SAF-T DocumentStatus.SourcePayment enum on receipts.
// Identical set as SourceBilling.
type SourcePayment = SourceBilling

// SpecialRegimes flags invoice-level regimes set in SAF-T SalesInvoice.SpecialRegimes.
type SpecialRegimes struct {
	SelfBilling  bool `json:"self_billing"`
	CashVAT      bool `json:"cash_vat"`
	ThirdParties bool `json:"third_parties"`
}

// IssueOptions carries caller-declared issuance intent. Status is NOT an option —
// it is always derived from the family + SpecialRegimes (see D-3 in FIX_PLAN.md).
// Cancellation is a separate transition (P2.6), not an issuance variant.
//
// Zero-value IssueOptions{} produces SourceBilling = "P" (Produced) and applies the
// CIVA Art. 36.º five-working-day emission guard with a weekend-only HolidayCalendar.
type IssueOptions struct {
	SourceBilling SourceBilling
	// Calendar overrides the weekend-only EmptyCalendar fallback (§0.5 in
	// FIX_PLAN.md). Pass a production calendar that knows national/regional
	// holidays once wired.
	Calendar HolidayCalendar
	// Recovery flips to true for documents ingested via manual / backup
	// recovery flows (Portaria 363/2010). Bypasses the 5-working-day emission
	// guard AND the monotonic-date-in-series guard, since recovery necessarily
	// ingests documents out of normal sequence (originals may pre-date prior
	// recovered entries).
	Recovery bool
	// Reader resolves cross-document references for invariants that need them
	// (e.g. ND product-set vs originating invoice). Required when issuing ND.
	Reader IssuedDocumentReader
	// FSLimits overrides DefaultFSLimits for the FS gross-total cap. Used only
	// when DocumentType == FS.
	FSLimits *FSLimits
	// IssuerEAC is the issuer Company's EAC code — drives FS retail-tier
	// resolution. Required when DocumentType == FS (use Company.EACCode).
	IssuerEAC string
}

// resolveSourceBilling defaults the zero value to Produced and validates the rest.
func (o IssueOptions) resolveSourceBilling() (SourceBilling, error) {
	if o.SourceBilling == "" {
		return SourceBillingProduced, nil
	}
	if !o.SourceBilling.IsValid() {
		return "", fmt.Errorf("invalid SourceBilling: %q", o.SourceBilling)
	}
	return o.SourceBilling, nil
}

// Signer produces the SAF-T document hash and hash control.
// The canonical input is mandated by Portaria 363/2010, Article 5:
//
//	InvoiceDate;SystemEntryDateTime;DocumentNumber;GrossTotal;PreviousHash
//
// Implementations apply RSA-SHA1 with the AT-certified private key and return
// base64 hash plus the HashControl string. Domain stays signer-agnostic.
type Signer interface {
	Sign(canonical string) (hash, control string, err error)
}

// IssuedDocument is the family-agnostic immutable record of an issued source document.
// Status is a string at this level; family-specific specializations (SalesInvoice,
// StockMovement, WorkDocument, Payment) validate it against the right enum.
//
// CONCURRENCY: Issue mutates *Series (LastNum, LastHash, LastDate, LastSystemDate, Version).
// The caller MUST serialize per Series.ID and wrap Issue + persistence in a single DB
// transaction; otherwise concurrent issuance corrupts the hash chain. Version is the
// optimistic-lock token to compare in UPDATE … WHERE version = ?.
type IssuedDocument struct {
	Number          DocNumber      `json:"number"`
	ATCUD           ATCUD          `json:"atcud"`
	Hash            Hash           `json:"hash"`
	HashControl     HashControl    `json:"hash_control"`
	SystemEntryDate time.Time      `json:"system_entry_date"`
	SourceID        string         `json:"source_id"`
	SourceBilling   SourceBilling  `json:"source_billing"`
	Period          Period         `json:"period"`
	Status          DocumentStatus `json:"status"`
	StatusDate      time.Time      `json:"status_date"`
	// Reason is the cancellation justification when Status == "A" (SAF-T DocumentStatus.Reason).
	Reason string `json:"reason,omitempty"`
	// BilledByInvoice references the consuming invoice when WorkDocument.MarkBilled flips Status to "F".
	BilledByInvoice *DocNumber `json:"billed_by_invoice,omitempty"`

	DocumentCore
	QRPayload string `json:"qr_payload,omitempty"`
}

// canonicalHashInput formats the AT-required signing input per Portaria 363/2010.
// GrossTotal is rendered at 2dp per AT spec.
func canonicalHashInput(invoiceDate, systemEntry time.Time, number string, gross Money, prevHash string) string {
	return strings.Join([]string{
		invoiceDate.Format("2006-01-02"),
		systemEntry.Format("2006-01-02T15:04:05"),
		number,
		gross.Format2DP(),
		prevHash,
	}, ";")
}

// validateIssueContext checks the cross-cutting issuance guards that any document family
// has to satisfy before consuming the next sequence number: the series must be ready to
// issue, its doc type must match the draft's, the system entry date can't precede the
// reference date, and a source id is mandatory. Shared by Issue and IssuePayment.
func validateIssueContext(series *Series, docType DocumentType, sourceID string, refDate, now time.Time) error {
	if !series.CanIssue() {
		return fmt.Errorf("series %q cannot issue (not registered or inactive)", series.ID)
	}
	if series.DocType != docType {
		return fmt.Errorf("series doc type %s does not match draft %s", series.DocType, docType)
	}
	if now.Before(refDate) {
		return fmt.Errorf("system entry date %s precedes draft date %s", now, refDate)
	}
	if sourceID == "" {
		return fmt.Errorf("source id is required")
	}
	return nil
}

// dateOnly drops the time-of-day so AT's calendar-day monotonicity rule
// (Portaria 195/2020, InvoiceDate non-decreasing within a series) is enforced
// regardless of the time component callers stamp onto draft.Date.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// nextDocIdentity derives the next sequence number and its DocNumber/ATCUD/Period
// from the series state. Shared by issueCommon and IssuePayment so the
// seq == LastNum+1 contract (see Series.AppendIssue) has a single source.
func nextDocIdentity(series *Series, docType DocumentType, date time.Time) (int, DocNumber, ATCUD, Period, error) {
	seq := series.LastNum + 1
	number, err := NewDocNumber(docType, series.ID, seq)
	if err != nil {
		return 0, DocNumber{}, "", 0, err
	}
	atcud, err := NewATCUD(*series, seq)
	if err != nil {
		return 0, DocNumber{}, "", 0, err
	}
	period, err := NewPeriod(int(date.Month()))
	if err != nil {
		return 0, DocNumber{}, "", 0, err
	}
	return seq, number, atcud, period, nil
}

// issueCommon advances the series counter and produces a signed IssuedDocument.
// Caller MUST have run draft.Validate() — not re-checked here so lines validate once.
// On error the series is untouched. Caller MUST serialize per Series.ID inside a transaction.
func issueCommon(draft *CommonDraftDocument, series *Series, signer Signer, sourceID string, now time.Time, opts IssueOptions) (IssuedDocument, error) {
	sourceBilling, err := opts.resolveSourceBilling()
	if err != nil {
		return IssuedDocument{}, err
	}
	// AT signs and exports in Europe/Lisbon wall time. Normalize both endpoints
	// here so canonical hash, Period, and stored timestamps all share that clock.
	date := draft.Date.In(lisbonLocation)
	sysEntry := now.In(lisbonLocation)

	if err := validateIssueContext(series, draft.DocumentType, sourceID, date, sysEntry); err != nil {
		return IssuedDocument{}, err
	}

	if !opts.Recovery {
		if series.LastDate != nil && dateOnly(date).Before(dateOnly(*series.LastDate)) {
			return IssuedDocument{}, fmt.Errorf("%w: %s < %s", ErrDateRegression, date.Format("2006-01-02"), series.LastDate.Format("2006-01-02"))
		}
		// CIVA Art. 36.º §2: 5-working-day cap applies to faturas (FT/FS/FR).
		// NC/ND are corrections under a different timing rule and are not gated here.
		if draft.DocumentType.IsFactura() {
			if days := workingDaysBetween(date, sysEntry, opts.Calendar); days > 5 {
				return IssuedDocument{}, fmt.Errorf("emission gap %d working days exceeds CIVA Art. 36.º limit of 5 (use recovery flow instead)", days)
			}
		}
	}

	draft.CalculateTotals()

	seq, number, atcud, period, err := nextDocIdentity(series, draft.DocumentType, date)
	if err != nil {
		return IssuedDocument{}, err
	}

	canonical := canonicalHashInput(date, sysEntry, number.Format(), draft.Totals.GrossTotal, series.LastHash)
	hashStr, controlStr, err := signer.Sign(canonical)
	if err != nil {
		return IssuedDocument{}, fmt.Errorf("sign: %w", err)
	}
	hash := Hash(hashStr)
	control := HashControl(controlStr)
	if err := hash.Validate(); err != nil {
		return IssuedDocument{}, fmt.Errorf("signer returned invalid hash: %w", err)
	}
	if err := control.Validate(); err != nil {
		return IssuedDocument{}, fmt.Errorf("signer returned invalid hash control: %w", err)
	}

	core := draft.DocumentCore
	core.Date = date
	issued := IssuedDocument{
		Number:          number,
		ATCUD:           atcud,
		Hash:            hash,
		HashControl:     control,
		SystemEntryDate: sysEntry,
		SourceID:        sourceID,
		SourceBilling:   sourceBilling,
		Period:          period,
		Status:          StatusNormal,
		StatusDate:      sysEntry,
		DocumentCore:    core,
	}

	series.AppendIssue(seq, hashStr, date, sysEntry)

	return issued, nil
}

// Cancel marks the document as cancelled (Status = "A") if the e-Fatura
// deadline has not passed. Deadline = day-5 of the month FOLLOWING the
// document's InvoiceDate at 23:59:59 Europe/Lisbon ([CONFIRMAR] §0.5 fallback).
// Past the deadline a recovery flow is required (Tier-3 module).
//
// QRPayload is intentionally NOT mutated — the QR encodes original issuance
// state and is reprinted verbatim regardless of subsequent status changes.
func (d *IssuedDocument) Cancel(reason string, at time.Time) error {
	switch d.Status {
	case StatusNormal, StatusSelfBilled, StatusThirdParty:
		// permitted source states
	case StatusCancelled:
		return fmt.Errorf("document already cancelled")
	default:
		// F (Billed) and R (Summary) cannot be cancelled in isolation —
		// they're terminal states from the AT-cert perspective.
		return fmt.Errorf("cannot cancel from status %q", d.Status)
	}
	if len(reason) > MaxLenCancellationReason {
		return fmt.Errorf("cancellation reason exceeds %d chars", MaxLenCancellationReason)
	}

	deadline := cancellationDeadline(d.Date)
	if time.Now().After(deadline) {
		return fmt.Errorf("%w: %s", ErrCancellationDeadlinePassed, deadline.Format(time.RFC3339))
	}
	d.Status = StatusCancelled
	d.Reason = reason
	d.StatusDate = at.In(lisbonLocation)
	return nil
}

func cancellationDeadline(docDate time.Time) time.Time {
	lisbon := docDate.In(lisbonLocation)
	year, month := lisbon.Year(), lisbon.Month()+1
	if month > time.December {
		month = time.January
		year++
	}
	return time.Date(year, month, 5, 23, 59, 59, 0, lisbonLocation)
}

type HolidayCalendar interface {
	IsHoliday(date time.Time) bool
}

type EmptyCalendar struct{}

func (EmptyCalendar) IsHoliday(time.Time) bool { return false }

func workingDaysBetween(start, end time.Time, cal HolidayCalendar) int {
	if cal == nil {
		cal = EmptyCalendar{}
	}
	s := dateOnly(start)
	e := dateOnly(end)
	if !s.Before(e) {
		return 0
	}
	days := 0
	for cur := s.AddDate(0, 0, 1); !cur.After(e); cur = cur.AddDate(0, 0, 1) {
		switch cur.Weekday() {
		case time.Saturday, time.Sunday:
			continue
		}
		if cal.IsHoliday(cur) {
			continue
		}
		days++
	}
	return days
}
