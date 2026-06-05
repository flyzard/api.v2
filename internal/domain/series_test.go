package domain

import (
	"errors"
	"testing"
	"time"
)

var seriesT0 = time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

func registeredSeries(t *testing.T) Series {
	t.Helper()
	s, err := NewSeries("S2026", FT)
	if err != nil {
		t.Fatalf("NewSeries: %v", err)
	}
	if err := s.RegisterWithAT("BCDFGH37", seriesT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	return s
}

func TestUnregisteredSeriesCannotIssue(t *testing.T) {
	s, err := NewSeries("S2026", FT)
	if err != nil {
		t.Fatalf("NewSeries: %v", err)
	}
	if s.CanIssue() {
		t.Fatal("unregistered series must not issue")
	}
	if s.ATStatus != SeriesPending {
		t.Fatalf("ATStatus = %s, want %s", s.ATStatus, SeriesPending)
	}
}

func TestRegisterActivatesSeries(t *testing.T) {
	s := registeredSeries(t)
	if !s.CanIssue() {
		t.Fatal("registered series must issue")
	}
	if s.ATStatus != SeriesActive {
		t.Fatalf("ATStatus = %s, want %s", s.ATStatus, SeriesActive)
	}
}

func TestFinalize(t *testing.T) {
	s := registeredSeries(t)
	s.AppendIssue(1, "hash1", seriesT0, seriesT0)

	if err := s.Finalize(seriesT0.AddDate(0, 6, 0)); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if s.ATStatus != SeriesFinalized {
		t.Fatalf("ATStatus = %s, want %s", s.ATStatus, SeriesFinalized)
	}
	if s.FinalizedAt == nil {
		t.Fatal("FinalizedAt not set")
	}
	if s.CanIssue() {
		t.Fatal("finalized series must not issue")
	}
}

func TestFinalizeRequiresActiveSeries(t *testing.T) {
	s, err := NewSeries("S2026", FT)
	if err != nil {
		t.Fatalf("NewSeries: %v", err)
	}
	if err := s.Finalize(seriesT0); !errors.Is(err, ErrSeriesNotActive) {
		t.Fatalf("Finalize on pending series: err = %v, want ErrSeriesNotActive", err)
	}
}

func TestFinalizeTwiceRejected(t *testing.T) {
	s := registeredSeries(t)
	if err := s.Finalize(seriesT0); err != nil {
		t.Fatalf("first Finalize: %v", err)
	}
	if err := s.Finalize(seriesT0); !errors.Is(err, ErrSeriesNotActive) {
		t.Fatalf("second Finalize: err = %v, want ErrSeriesNotActive", err)
	}
}

func TestCancelUnusedSeries(t *testing.T) {
	s := registeredSeries(t)
	if err := s.Cancel(seriesT0); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if s.ATStatus != SeriesCancelled {
		t.Fatalf("ATStatus = %s, want %s", s.ATStatus, SeriesCancelled)
	}
	if s.CancelledAt == nil {
		t.Fatal("CancelledAt not set")
	}
	if s.CanIssue() {
		t.Fatal("cancelled series must not issue")
	}
}

func TestCancelWithIssuedDocumentsRejected(t *testing.T) {
	s := registeredSeries(t)
	s.AppendIssue(1, "hash1", seriesT0, seriesT0)

	if err := s.Cancel(seriesT0); !errors.Is(err, ErrSeriesHasDocuments) {
		t.Fatalf("Cancel after issue: err = %v, want ErrSeriesHasDocuments", err)
	}
	if !s.CanIssue() {
		t.Fatal("failed Cancel must leave series active")
	}
}

func TestCancelRequiresActiveSeries(t *testing.T) {
	s := registeredSeries(t)
	if err := s.Finalize(seriesT0); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := s.Cancel(seriesT0); !errors.Is(err, ErrSeriesNotActive) {
		t.Fatalf("Cancel on finalized series: err = %v, want ErrSeriesNotActive", err)
	}
}
