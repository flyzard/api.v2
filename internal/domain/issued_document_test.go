package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// XSD DocumentStatus Reason is SAFPTtextTypeMandatoryMax50Car — a reason
// accepted by the domain must never fail XSD validation at export time.
func TestCancel_ReasonCappedAt50Chars(t *testing.T) {
	recent := time.Date(2026, 6, 1, 9, 0, 0, 0, lisbonLocation)

	t.Run("51_chars_rejected", func(t *testing.T) {
		d := IssuedDocument{
			Status:       StatusNormal,
			DocumentCore: DocumentCore{Date: recent},
		}
		if err := d.Cancel(strings.Repeat("x", 51), recent); err == nil {
			t.Fatal("Cancel accepted 51-char reason; XSD Reason max is 50")
		}
	})

	t.Run("50_chars_accepted", func(t *testing.T) {
		d := IssuedDocument{
			Status:       StatusNormal,
			DocumentCore: DocumentCore{Date: recent},
		}
		if err := d.Cancel(strings.Repeat("x", 50), recent); err != nil {
			t.Fatalf("Cancel rejected 50-char reason: %v", err)
		}
	})
}

// The e-Fatura deadline must be evaluated against the caller-supplied
// cancellation instant (at), not the wall clock — StatusDate already uses
// at, and a wall-clock check makes replay/tests nondeterministic.
func TestCancel_DeadlineUsesAtParam(t *testing.T) {
	docDate := time.Date(2026, 1, 10, 9, 0, 0, 0, lisbonLocation)
	// deadline for a January doc = Feb 5 23:59:59

	t.Run("at_before_deadline_succeeds", func(t *testing.T) {
		d := IssuedDocument{
			Status:       StatusNormal,
			DocumentCore: DocumentCore{Date: docDate},
		}
		at := time.Date(2026, 1, 20, 12, 0, 0, 0, lisbonLocation)
		if err := d.Cancel("Erro de emissão", at); err != nil {
			t.Fatalf("Cancel at %s (before Feb 5 deadline) rejected: %v", at, err)
		}
		if !d.StatusDate.Equal(at) {
			t.Fatalf("StatusDate = %s, want %s", d.StatusDate, at)
		}
	})

	t.Run("at_after_deadline_rejected", func(t *testing.T) {
		d := IssuedDocument{
			Status:       StatusNormal,
			DocumentCore: DocumentCore{Date: docDate},
		}
		at := time.Date(2026, 3, 1, 12, 0, 0, 0, lisbonLocation)
		if err := d.Cancel("Erro de emissão", at); err == nil {
			t.Fatal("Cancel at March 1 accepted; deadline was Feb 5")
		}
	})

	t.Run("december_deadline_rolls_into_next_year", func(t *testing.T) {
		d := IssuedDocument{
			Status:       StatusNormal,
			DocumentCore: DocumentCore{Date: time.Date(2026, 12, 20, 9, 0, 0, 0, lisbonLocation)},
		}
		at := time.Date(2027, 1, 4, 12, 0, 0, 0, lisbonLocation) // before Jan 5 deadline
		if err := d.Cancel("Erro de emissão", at); err != nil {
			t.Fatalf("Cancel before Jan 5 next-year deadline rejected: %v", err)
		}
	})
}

// SystemEntryDate must never regress within a series — AT orders SAF-T
// entries by number and expects non-decreasing SystemEntryDate. The series
// already records LastSystemDate; issuance must reject a clock that runs
// backwards. Applies to recovery too: sysEntry is the real server clock.
func TestIssue_SystemEntryRegressionRejected(t *testing.T) {
	docDate := time.Date(2026, 6, 1, 0, 0, 0, 0, lisbonLocation)

	t.Run("earlier_clock_rejected", func(t *testing.T) {
		s := registeredSeries(t)
		last := time.Date(2026, 6, 2, 10, 0, 0, 0, lisbonLocation)
		s.LastSystemDate = &last
		now := last.Add(-time.Minute)
		err := validateIssueContext(&s, FT, "op-1", docDate, now, false)
		if !errors.Is(err, ErrSystemEntryRegression) {
			t.Fatalf("err = %v, want ErrSystemEntryRegression", err)
		}
	})

	t.Run("equal_clock_accepted", func(t *testing.T) {
		s := registeredSeries(t)
		last := time.Date(2026, 6, 2, 10, 0, 0, 0, lisbonLocation)
		s.LastSystemDate = &last
		if err := validateIssueContext(&s, FT, "op-1", docDate, last, false); err != nil {
			t.Fatalf("same-instant issue rejected: %v", err)
		}
	})

	t.Run("later_clock_accepted", func(t *testing.T) {
		s := registeredSeries(t)
		last := time.Date(2026, 6, 2, 10, 0, 0, 0, lisbonLocation)
		s.LastSystemDate = &last
		if err := validateIssueContext(&s, FT, "op-1", docDate, last.Add(time.Second), false); err != nil {
			t.Fatalf("later issue rejected: %v", err)
		}
	})
}

// TestIssue_DateBeforeSeriesRegistration pins series-communicated-before-use
// (docs/series-rules.yaml; Portaria 195/2020 art. 5.º): a normal document
// cannot be dated before the series was communicated to AT. Recovery is exempt
// — it re-issues documents whose original dates legally precede the recovery
// series' registration. Comparison is calendar-day in Europe/Lisbon.
func TestIssue_DateBeforeSeriesRegistration(t *testing.T) {
	now := seriesT0.Add(48 * time.Hour)

	t.Run("normal_backdated_rejected", func(t *testing.T) {
		s := registeredSeries(t) // registered at seriesT0 = 2026-06-01
		docDate := time.Date(2026, 5, 28, 0, 0, 0, 0, lisbonLocation)
		err := validateIssueContext(&s, FT, "op-1", docDate, now, false)
		if !errors.Is(err, ErrDateBeforeRegistration) {
			t.Fatalf("err = %v, want ErrDateBeforeRegistration", err)
		}
	})

	t.Run("registration_day_accepted", func(t *testing.T) {
		s := registeredSeries(t)
		// Same legal day as registration (2026-06-01 10:00 UTC), earlier wall clock.
		docDate := time.Date(2026, 6, 1, 0, 0, 0, 0, lisbonLocation)
		if err := validateIssueContext(&s, FT, "op-1", docDate, now, false); err != nil {
			t.Fatalf("registration-day issue rejected: %v", err)
		}
	})

	t.Run("nil_registration_date_fails_closed", func(t *testing.T) {
		// A registered-active series always has RegistrationDate in-memory
		// (RegisterWithAT sets both), but a rehydrated Series can lose it
		// (json omitempty). The guard must fail closed, not silently skip.
		s := registeredSeries(t)
		s.RegistrationDate = nil
		docDate := time.Date(2026, 6, 2, 0, 0, 0, 0, lisbonLocation)
		err := validateIssueContext(&s, FT, "op-1", docDate, now, false)
		if !errors.Is(err, ErrMissingRegistrationDate) {
			t.Fatalf("err = %v, want ErrMissingRegistrationDate", err)
		}
	})

	t.Run("recovery_backdated_accepted", func(t *testing.T) {
		s := mustVal(NewRecoverySeries("R2026", FT))
		if err := s.RegisterWithAT("BCDFGH37", seriesT0); err != nil {
			t.Fatalf("RegisterWithAT: %v", err)
		}
		docDate := time.Date(2026, 5, 1, 0, 0, 0, 0, lisbonLocation)
		if err := validateIssueContext(&s, FT, "op-1", docDate, now, true); err != nil {
			t.Fatalf("recovery issue with pre-registration date rejected: %v", err)
		}
	})
}

// OU ("outros documentos") is a legal WorkType in the XSD — covers documents
// presented to the customer for conference that fit no other code.
func TestOUWorkTypeValid(t *testing.T) {
	if !OU.IsValid() {
		t.Fatal("OU must be a valid document type")
	}
	if !OU.IsWorking() {
		t.Fatal("OU must belong to the working family")
	}
}

// Payment.Cancel shares validateCancellation — same at-based deadline rule,
// anchored on TransactionDate.
func TestPaymentCancel_DeadlineUsesAtParam(t *testing.T) {
	txDate := time.Date(2026, 1, 10, 9, 0, 0, 0, lisbonLocation)

	p := Payment{
		Status:          StatusNormal,
		TransactionDate: txDate,
	}
	at := time.Date(2026, 1, 20, 12, 0, 0, 0, lisbonLocation)
	if err := p.Cancel("Erro de emissão", at); err != nil {
		t.Fatalf("Cancel at %s (before Feb 5 deadline) rejected: %v", at, err)
	}
}
