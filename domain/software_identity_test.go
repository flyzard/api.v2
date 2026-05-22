package domain

import (
	"strings"
	"testing"
)

func validSoftwareIdentity() SoftwareIdentity {
	return SoftwareIdentity{
		ProducerTaxID:     "519348761",
		SoftwareName:      "Faturly",
		ProducerName:      "AVENIDA DO CODIGO LDA",
		Version:           "1.0.0",
		CertificateNumber: "9999",
	}
}

func TestSoftwareIdentity_Valid(t *testing.T) {
	if _, err := NewSoftwareIdentity(validSoftwareIdentity()); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
}

func TestSoftwareIdentity_RejectsBadProducerTaxID(t *testing.T) {
	s := validSoftwareIdentity()
	s.ProducerTaxID = "000000000"
	if _, err := NewSoftwareIdentity(s); err == nil {
		t.Fatal("invalid producer NIF should be rejected")
	}
}

func TestSoftwareIdentity_RejectsEmptyVersion(t *testing.T) {
	s := validSoftwareIdentity()
	s.Version = ""
	if _, err := NewSoftwareIdentity(s); err == nil {
		t.Fatal("empty version should be rejected")
	}
}

func TestSoftwareIdentity_RejectsLongCertificate(t *testing.T) {
	s := validSoftwareIdentity()
	s.CertificateNumber = strings.Repeat("9", 11)
	if _, err := NewSoftwareIdentity(s); err == nil {
		t.Fatal("11-char certificate number should be rejected")
	}
}

func TestSoftwareIdentity_ProductIDFormat(t *testing.T) {
	s := validSoftwareIdentity()
	if got, want := s.ProductID(), "Faturly/AVENIDA DO CODIGO LDA"; got != want {
		t.Errorf("ProductID: got %q, want %q", got, want)
	}
}
