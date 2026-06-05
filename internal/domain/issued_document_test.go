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
