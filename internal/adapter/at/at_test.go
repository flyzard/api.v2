package at

import (
	"strings"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

var atT0 = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func TestRegistrationForNormalSeries(t *testing.T) {
	s, err := domain.NewSeries("S2026", domain.FT)
	if err != nil {
		t.Fatalf("NewSeries: %v", err)
	}
	reg, err := RegistrationFor(s, atT0)
	if err != nil {
		t.Fatalf("RegistrationFor: %v", err)
	}
	if reg.SeriesType != "N" {
		t.Errorf("SeriesType = %q, want N", reg.SeriesType)
	}
	if reg.InitialSeq != 1 {
		t.Errorf("InitialSeq = %d, want 1", reg.InitialSeq)
	}
	if reg.DocType != domain.FT || reg.SeriesID != "S2026" {
		t.Errorf("identity mismatch: %+v", reg)
	}
}

func TestRegistrationForRecoverySeries(t *testing.T) {
	s, err := domain.NewRecoverySeries("REC2026", domain.FT)
	if err != nil {
		t.Fatalf("NewRecoverySeries: %v", err)
	}
	reg, err := RegistrationFor(s, atT0)
	if err != nil {
		t.Fatalf("RegistrationFor: %v", err)
	}
	if reg.SeriesType != "R" {
		t.Errorf("SeriesType = %q, want R", reg.SeriesType)
	}
}

func TestRegistrationForAlreadyRegistered(t *testing.T) {
	s, _ := domain.NewSeries("S2026", domain.FT)
	if err := s.RegisterWithAT("BCDFGH37", atT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	if _, err := RegistrationFor(s, atT0); err == nil {
		t.Fatal("want error for already-registered series")
	}
}

func TestFinalizationFor(t *testing.T) {
	s, _ := domain.NewSeries("S2026", domain.FT)
	if err := s.RegisterWithAT("BCDFGH37", atT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	s.AppendIssue(7, "h", atT0, atT0)

	fin, err := FinalizationFor(s, "switching software")
	if err != nil {
		t.Fatalf("FinalizationFor: %v", err)
	}
	if fin.ATCode != "BCDFGH37" || fin.LastSeq != 7 || fin.Justification != "switching software" {
		t.Errorf("unexpected finalization: %+v", fin)
	}
}

func TestFinalizationForUnregistered(t *testing.T) {
	s, _ := domain.NewSeries("S2026", domain.FT)
	if _, err := FinalizationFor(s, ""); err == nil {
		t.Fatal("want error for unregistered series")
	}
}

func TestFinalizationForUnusedSeriesRejected(t *testing.T) {
	s, _ := domain.NewSeries("S2026", domain.FT)
	if err := s.RegisterWithAT("BCDFGH37", atT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	if _, err := FinalizationFor(s, ""); err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("err = %v, want cancel-instead error (seqUltimoDocEmitido must be >= 1)", err)
	}
}

func TestCancellationFor(t *testing.T) {
	s, _ := domain.NewSeries("S2026", domain.FT)
	if err := s.RegisterWithAT("BCDFGH37", atT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	c, err := CancellationFor(s)
	if err != nil {
		t.Fatalf("CancellationFor: %v", err)
	}
	if c.Reason != CancelReasonError {
		t.Errorf("Reason = %q, want %q", c.Reason, CancelReasonError)
	}
}

func TestCancellationForUsedSeriesRejected(t *testing.T) {
	s, _ := domain.NewSeries("S2026", domain.FT)
	if err := s.RegisterWithAT("BCDFGH37", atT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	s.AppendIssue(1, "h", atT0, atT0)
	if _, err := CancellationFor(s); err == nil || !strings.Contains(err.Error(), "finalize") {
		t.Fatalf("err = %v, want finalize-instead error", err)
	}
}

func TestDocClass(t *testing.T) {
	cases := map[domain.DocumentType]string{
		domain.FT: "SI",
		domain.GT: "MG",
		domain.OR: "WD",
		domain.RC: "PY",
	}
	for dt, want := range cases {
		got, err := docClass(dt)
		if err != nil || got != want {
			t.Errorf("docClass(%s) = %q, %v; want %q", dt, got, err, want)
		}
	}
	if _, err := docClass(domain.DocumentType("XX")); err == nil {
		t.Error("docClass(XX): want error")
	}
}

func TestStatusFromEstado(t *testing.T) {
	cases := map[string]domain.SeriesATStatus{
		"A": domain.SeriesActive,
		"F": domain.SeriesFinalized,
		"N": domain.SeriesCancelled,
		"?": domain.SeriesPending,
	}
	for estado, want := range cases {
		if got := statusFromEstado(estado); got != want {
			t.Errorf("statusFromEstado(%q) = %s, want %s", estado, got, want)
		}
	}
}
