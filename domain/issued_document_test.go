package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeSigner deterministically derives a hash from (prevHash, canonical) so tests can
// assert chain linkage without depending on RSA keys.
type fakeSigner struct {
	control string
	failOn  string
}

func (f fakeSigner) Sign(prevHash, canonical string) (string, string, error) {
	if f.failOn != "" && strings.Contains(canonical, f.failOn) {
		return "", "", errors.New("signer disabled")
	}
	sum := sha256.Sum256([]byte(prevHash + "|" + canonical))
	return base64.StdEncoding.EncodeToString(sum[:]), f.control, nil
}

func registeredSeries(t *testing.T, id string, dt DocumentType) *Series {
	t.Helper()
	s, err := NewSeries(id, dt)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterWithAT("AAJ7H5K2", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return &s
}

func validDraft(t *testing.T) *CommonDraftDocument {
	t.Helper()
	addr, _ := NewAddress("Rua A 1", "Lisboa", "1000-001", "PT")
	cust, err := NewCustomer("ACC1", "503504564", "Acme", addr, false)
	if err != nil {
		t.Fatal(err)
	}
	user, err := NewUser("user@example.com", "User")
	if err != nil {
		t.Fatal(err)
	}
	tax, _ := NewVATLineTax(PT, TaxNormal, "", "")
	product := Product{ProductCode: "PROD-1", ProductDescription: "Default test product"}
	line := DocumentLine{
		LineNumber:   1,
		Product:      product,
		Description:  product.ProductDescription,
		Quantity:     mustQuantity(t, 1),
		UnitPrice:    mustMoney(t, 100),
		TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Tax:          tax,
	}
	return &CommonDraftDocument{
		DocumentCore: DocumentCore{
			DocumentType: FT,
			Customer:     *cust,
			Date:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			IssuedBy:     user,
			Lines:        []DocumentLine{line},
		},
	}
}

func TestCalculateTotalsVATOnly(t *testing.T) {
	d := validDraft(t)
	d.CalculateTotals()
	if d.Totals.NetTotal != mustMoney(t, 100) {
		t.Errorf("NetTotal: got %v want 100", d.Totals.NetTotal)
	}
	if d.Totals.TaxTotal != mustMoney(t, 23) {
		t.Errorf("TaxTotal: got %v want 23", d.Totals.TaxTotal)
	}
	if d.Totals.StampDuty != 0 {
		t.Errorf("StampDuty: got %v want 0", d.Totals.StampDuty)
	}
	if d.Totals.GrossTotal != mustMoney(t, 123) {
		t.Errorf("GrossTotal: got %v want 123", d.Totals.GrossTotal)
	}
}

func TestCalculateTotalsMixedVATAndStamp(t *testing.T) {
	d := validDraft(t)
	stampAmt := mustMoney(t, 5)
	stampTax, _ := NewStampLineTax("PT", "IS-G", stampAmt)
	stampLine := DocumentLine{
		LineNumber:   2,
		Quantity:     mustQuantity(t, 1),
		UnitPrice:    mustMoney(t, 50),
		TaxPointDate: d.Date,
		Tax:          stampTax,
	}
	d.Lines = append(d.Lines, stampLine)
	d.CalculateTotals()
	if d.Totals.NetTotal != mustMoney(t, 150) {
		t.Errorf("NetTotal: got %v want 150", d.Totals.NetTotal)
	}
	if d.Totals.TaxTotal != mustMoney(t, 23) {
		t.Errorf("TaxTotal: got %v want 23", d.Totals.TaxTotal)
	}
	if d.Totals.StampDuty != stampAmt {
		t.Errorf("StampDuty: got %v want %v", d.Totals.StampDuty, stampAmt)
	}
	if d.Totals.GrossTotal != mustMoney(t, 178) {
		t.Errorf("GrossTotal: got %v want 178", d.Totals.GrossTotal)
	}
}

func TestIssueHappyPath(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 30, 0, 0, time.UTC)
	signer := fakeSigner{control: "1"}

	issued, err := issueCommon(d, s, signer, "user-1", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Number.Seq != 1 {
		t.Errorf("Seq: got %d want 1", issued.Number.Seq)
	}
	if issued.Number.Format() != "FT A/1" {
		t.Errorf("Format: got %q", issued.Number.Format())
	}
	if string(issued.ATCUD) != "AAJ7H5K2-1" {
		t.Errorf("ATCUD: got %q", issued.ATCUD)
	}
	if issued.Period != 1 {
		t.Errorf("Period: got %v want 1", issued.Period)
	}
	if issued.Status != StatusNormal {
		t.Errorf("Status: got %q", issued.Status)
	}
	if s.LastNum != 1 {
		t.Errorf("series LastNum: got %d want 1", s.LastNum)
	}
	if s.LastHash == "" {
		t.Error("series LastHash should be set after issue")
	}
	if s.LastSystemDate == nil || !s.LastSystemDate.Equal(now) {
		t.Error("series LastSystemDate not advanced")
	}
}

func TestIssueRejectsUnregisteredSeries(t *testing.T) {
	d := validDraft(t)
	s, _ := NewSeries("A", FT) // not registered with AT
	d.Series = s
	_, err := issueCommon(d, &s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected error for unregistered series")
	}
}

func TestIssueRejectsMismatchedDocType(t *testing.T) {
	d := validDraft(t)        // FT
	s := registeredSeries(t, "B", NC) // NC series
	d.Series = *s
	_, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", time.Now(), IssueOptions{})
	if err == nil {
		t.Fatal("expected error for series doc type mismatch")
	}
}

func TestIssueRejectsSystemEntryBeforeInvoiceDate(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	past := d.Date.Add(-time.Hour)
	_, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", past, IssueOptions{})
	if err == nil {
		t.Fatal("expected error for system entry before invoice date")
	}
}

func TestIssue_RejectsBackDated(t *testing.T) {
	s := registeredSeries(t, "A", FT)
	signer := fakeSigner{control: "1"}

	first := validDraft(t)
	first.Series = *s
	first.Date = time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	if _, err := issueCommon(first, s, signer, "u", first.Date, IssueOptions{}); err != nil {
		t.Fatalf("first issue: %v", err)
	}

	second := validDraft(t)
	second.Series = *s
	second.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) // earlier day
	_, err := issueCommon(second, s, signer, "u", time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC), IssueOptions{})
	if !errors.Is(err, ErrDateRegression) {
		t.Fatalf("want ErrDateRegression, got %v", err)
	}
}

func TestIssue_SameDayAllowedAfterLateIssue(t *testing.T) {
	// Regression: prior literal plan used LastSystemDate as the proxy,
	// which falsely rejected next-day invoices when the prior was issued late.
	s := registeredSeries(t, "A", FT)
	signer := fakeSigner{control: "1"}

	first := validDraft(t)
	first.Series = *s
	first.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	// issued two days late
	if _, err := issueCommon(first, s, signer, "u", time.Date(2026, 1, 17, 14, 0, 0, 0, time.UTC), IssueOptions{}); err != nil {
		t.Fatalf("first issue: %v", err)
	}

	second := validDraft(t)
	second.Series = *s
	second.Date = time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC) // > prior draft.Date, < prior LastSystemDate
	if _, err := issueCommon(second, s, signer, "u", time.Date(2026, 1, 17, 15, 0, 0, 0, time.UTC), IssueOptions{}); err != nil {
		t.Fatalf("second issue must succeed: %v", err)
	}
}

func TestIssue_NormalizesToLisbon(t *testing.T) {
	// Two issuances differing only by the caller's timezone label must produce
	// the same canonical hash input — Issue normalizes both endpoints to
	// Europe/Lisbon before signing.
	utcNow := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC) // summer → Lisbon = UTC+1
	estLoc := time.FixedZone("EST", -5*3600)
	estNow := utcNow.In(estLoc) // same instant, different label

	build := func(t *testing.T, now time.Time) Hash {
		t.Helper()
		d := validDraft(t)
		s := registeredSeries(t, "A", FT)
		d.Series = *s
		d.Date = time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
		// Force the draft date to also vary in caller location to prove normalization.
		d.Date = d.Date.In(now.Location())
		// Update line tax-point-date to stay valid.
		for i := range d.Lines {
			d.Lines[i].TaxPointDate = d.Date
		}
		issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		return issued.Hash
	}

	if build(t, utcNow) != build(t, estNow) {
		t.Fatal("hash diverged between callers in UTC and EST despite same instant")
	}
}

func TestIssue_IncrementsSeriesVersion(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	if _, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", now, IssueOptions{}); err != nil {
		t.Fatal(err)
	}
	if s.Version != 1 {
		t.Errorf("Version after 1st issue: got %d, want 1", s.Version)
	}
	d2 := validDraft(t)
	d2.Series = *s
	if _, err := issueCommon(d2, s, fakeSigner{control: "1"}, "u", now.Add(time.Hour), IssueOptions{}); err != nil {
		t.Fatal(err)
	}
	if s.Version != 2 {
		t.Errorf("Version after 2nd issue: got %d, want 2", s.Version)
	}
}

func TestIssue_RejectsLateEmission(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	d.Date = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
	// > 5 working days later
	now := time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC) // Tuesday next week (6 working days)
	_, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", now, IssueOptions{})
	if err == nil {
		t.Fatal("expected emission guard error")
	}
}

func TestIssue_AcceptsFiveWorkingDayGap(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	d.Date = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
	now := time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC) // Monday next week (5 working days)
	if _, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", now, IssueOptions{}); err != nil {
		t.Fatalf("5 working days gap should pass: %v", err)
	}
}

func TestIssue_LateNCNotGuarded(t *testing.T) {
	// NC is a correction note — CIVA Art. 36 §2 5-day rule does not apply.
	// First, land an originating invoice on Jan 5 so the NC reference resolves
	// to something (NC.Validate requires References on every line).
	s := registeredSeries(t, "A", NC)
	signer := fakeSigner{control: "1"}
	d := validDraft(t)
	d.DocumentType = NC
	d.Series = *s
	d.Date = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for i := range d.Lines {
		d.Lines[i].References = []DocReference{{Reference: "FT A/1", Reason: "value adjustment"}}
	}
	now := time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC) // 6 working days later
	if _, err := issueCommon(d, s, signer, "u", now, IssueOptions{}); err != nil {
		t.Fatalf("NC should not be subject to 5-day guard: %v", err)
	}
}

func TestWorkingDays_SkipsWeekends(t *testing.T) {
	mon := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	nextMon := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	if got := workingDaysBetween(mon, nextMon, nil); got != 5 {
		t.Errorf("Mon→Mon = %d working days, want 5", got)
	}
}

func TestIssue_RecoveryBypassesEmissionGuard(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	d.Date = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC) // > 5 working days
	if _, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", now, IssueOptions{Recovery: true}); err != nil {
		t.Fatalf("Recovery should allow late issue: %v", err)
	}
}

func TestIssue_RecoveryBypassesMonotonicDate(t *testing.T) {
	s := registeredSeries(t, "A", FT)
	signer := fakeSigner{control: "1"}
	first := validDraft(t)
	first.Series = *s
	first.Date = time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	if _, err := issueCommon(first, s, signer, "u", first.Date, IssueOptions{}); err != nil {
		t.Fatal(err)
	}
	second := validDraft(t)
	second.Series = *s
	second.Date = time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC) // way before first
	if _, err := issueCommon(second, s, signer, "u", time.Date(2026, 1, 16, 14, 0, 0, 0, time.UTC), IssueOptions{Recovery: true}); err != nil {
		t.Fatalf("Recovery should allow back-dated entry: %v", err)
	}
}

// fakeClock returns a fixed instant — used by Cancel deadline tests.
type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

func TestCancel_BeforeDeadline_Succeeds(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Cancel called 10 days later, well before Feb 5 deadline.
	clk := fakeClock{t: time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC)}
	if err := issued.Cancel("customer changed mind", clk.t, clk); err != nil {
		t.Fatalf("cancel should succeed: %v", err)
	}
	if issued.Status != StatusCancelled {
		t.Errorf("status: got %q, want %q", issued.Status, StatusCancelled)
	}
	if issued.Reason != "customer changed mind" {
		t.Errorf("reason not stored")
	}
}

func TestCancel_AfterDeadline_Errors(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Cancel after Feb 5 23:59:59 Lisbon — should fail.
	clk := fakeClock{t: time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC)}
	err = issued.Cancel("too late", clk.t, clk)
	if !errors.Is(err, ErrCancellationDeadlinePassed) {
		t.Fatalf("want ErrCancellationDeadlinePassed, got %v", err)
	}
}

func TestCancel_DeadlineComputedInLisbon(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	d.Date = time.Date(2026, 1, 31, 23, 0, 0, 0, time.UTC) // late Jan UTC = late Jan Lisbon
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", d.Date, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Feb 5 23:59:00 Lisbon — within deadline.
	clk := fakeClock{t: time.Date(2026, 2, 5, 23, 0, 0, 0, time.UTC)}
	if err := issued.Cancel("ok", clk.t, clk); err != nil {
		t.Fatalf("Feb 5 within deadline should succeed: %v", err)
	}
}

func TestCancel_RejectsBilledStatus(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	issued.Status = StatusBilled // simulate a billed transition
	clk := fakeClock{t: time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)}
	if err := issued.Cancel("oops", clk.t, clk); err == nil {
		t.Fatal("cancel from Billed status should be rejected")
	}
}

func TestCancel_RejectsSummaryStatus(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	issued.Status = StatusSummary
	clk := fakeClock{t: time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)}
	if err := issued.Cancel("oops", clk.t, clk); err == nil {
		t.Fatal("cancel from Summary status should be rejected")
	}
}

func TestCancel_DoesNotMutateQRPayload(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	issued, err := issueCommon(d, s, fakeSigner{control: "1"}, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	issued.QRPayload = "A:507223606*B:518627878*C:PT*D:FT*..."
	original := issued.QRPayload
	clk := fakeClock{t: time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)}
	if err := issued.Cancel("oops", clk.t, clk); err != nil {
		t.Fatal(err)
	}
	if issued.QRPayload != original {
		t.Fatal("Cancel must not mutate QRPayload")
	}
}

func TestIssueHashChain(t *testing.T) {
	s := registeredSeries(t, "A", FT)
	signer := fakeSigner{control: "1"}

	d1 := validDraft(t)
	d1.Series = *s
	now := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
	first, err := issueCommon(d1, s, signer, "u", now, IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}

	d2 := validDraft(t)
	d2.Series = *s
	second, err := issueCommon(d2, s, signer, "u", now.Add(time.Minute), IssueOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if first.Hash == second.Hash {
		t.Fatal("second hash should differ from first (chain depends on prev)")
	}
	if second.Number.Seq != 2 {
		t.Errorf("second Seq: got %d want 2", second.Number.Seq)
	}
	if string(second.ATCUD) != "AAJ7H5K2-2" {
		t.Errorf("second ATCUD: got %q", second.ATCUD)
	}
}

func TestIssueDoesNotMutateOnSignerError(t *testing.T) {
	d := validDraft(t)
	s := registeredSeries(t, "A", FT)
	d.Series = *s
	signer := fakeSigner{control: "1", failOn: "FT A/1"}

	_, err := issueCommon(d, s, signer, "u", time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), IssueOptions{})
	if err == nil {
		t.Fatal("expected error from signer")
	}
	if s.LastNum != 0 {
		t.Errorf("series LastNum mutated on error: %d", s.LastNum)
	}
	if s.LastHash != "" {
		t.Errorf("series LastHash mutated on error: %q", s.LastHash)
	}
}

func TestIssue_HashChainDependsOnPrevHash(t *testing.T) {
	// Same body, different prevHash → different canonical input → different hash.
	// This pins the AT chain invariant: prevHash must materially influence the
	// signature, not be a vestigial parameter.
	d := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	sys := time.Date(2026, 1, 16, 10, 30, 0, 0, time.UTC)
	gross, _ := NewMoney(100)
	a := canonicalHashInput(d, sys, "FT A/1", gross, "PREV-A")
	b := canonicalHashInput(d, sys, "FT A/1", gross, "PREV-B")
	if a == b {
		t.Fatal("canonical input must change when prevHash changes")
	}
}

func TestValidate_Idempotent(t *testing.T) {
	d := validDraft(t)
	d.Series = *registeredSeries(t, "A", FT)
	tax, _ := NewVATLineTax(PT, TaxNormal, "", "")
	d.AddLine(DocumentLine{
		LineNumber:   99, // caller value overwritten by AddLine
		Product:      Product{ProductCode: "X", ProductDescription: "Y"},
		Description:  "Y",
		Quantity:     mustQuantity(t, 1),
		UnitPrice:    mustMoney(t, 10),
		TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Tax:          tax,
	})
	before := len(d.Lines)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(d.Lines) != before {
		t.Errorf("Validate mutated Lines: %d → %d", before, len(d.Lines))
	}
}

func TestCanonicalHashInputFormat(t *testing.T) {
	d := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	sys := time.Date(2026, 1, 16, 10, 30, 0, 0, time.UTC)
	gross, _ := NewMoney(123.45)
	got := canonicalHashInput(d, sys, "FT A/1", gross, "PREV")
	want := "2026-01-15;2026-01-16T10:30:00;FT A/1;123.45;PREV"
	if got != want {
		t.Errorf("canonical:\n got  %q\n want %q", got, want)
	}
}

func TestStatusValidForFamily(t *testing.T) {
	if !StatusNormal.ValidFor(FT) || !StatusSelfBilled.ValidFor(FT) {
		t.Fatal("FT should accept Normal and SelfBilled")
	}
	if StatusSelfBilled.ValidFor(GT) {
		t.Fatal("transport should not accept SelfBilled")
	}
	if !StatusThirdParty.ValidFor(GT) {
		t.Fatal("transport should accept ThirdParty")
	}
	if StatusThirdParty.ValidFor(FT) {
		t.Fatal("sales should not accept ThirdParty")
	}
	if StatusSummary.ValidFor(OR) {
		t.Fatal("working should not accept Summary")
	}
	if !StatusCancelled.ValidFor(RC) {
		t.Fatal("receipts should accept Cancelled")
	}
	if DocumentStatus("X").ValidFor(FT) {
		t.Fatal("unknown status should be invalid")
	}
	if !SourceBillingProduced.IsValid() {
		t.Fatal("SourceBillingProduced should be valid")
	}
}
