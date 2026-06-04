// Package config carries deployment/build settings sourced from the
// environment (and an optional .env file), as opposed to domain.v2 business
// entities. SoftwareIdentity is the AT-certified producer/software metadata
// stamped into every SAF-T Header — identical for every invoice the binary
// issues, so it lives here rather than in the domain model.
package config

import (
	"fmt"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// SoftwareIdentity is the producer/software metadata required by SAF-T
// AuditFile/Header. Values come from config (see Load), not the domain.
type SoftwareIdentity struct {
	ProducerTaxID     string
	SoftwareName      string
	ProducerName      string
	Version           string
	CertificateNumber string
}

// Validate enforces the SAF-T field constraints, reusing the domain's NIF
// checksum so config and domain agree on what AT will accept. The Windows-1252
// charset is enforced at SAF-T byte emission (see saft.transcodeWin1252).
func (s SoftwareIdentity) Validate() error {
	if !domain.TaxID(s.ProducerTaxID).IsValid() {
		return fmt.Errorf("invalid producer tax id: %q", s.ProducerTaxID)
	}
	if n := len(s.SoftwareName); n < 1 || n > 50 {
		return fmt.Errorf("software name length must be 1..50, got %d", n)
	}
	if n := len(s.ProducerName); n < 1 || n > 100 {
		return fmt.Errorf("producer name length must be 1..100, got %d", n)
	}
	if n := len(s.Version); n < 1 || n > 30 {
		return fmt.Errorf("version length must be 1..30, got %d", n)
	}
	if n := len(s.CertificateNumber); n < 1 || n > 10 {
		return fmt.Errorf("certificate number length must be 1..10, got %d", n)
	}
	return nil
}

// ProductID is the SAF-T Header.ProductID format — "SoftwareName/ProducerName".
func (s SoftwareIdentity) ProductID() string {
	return s.SoftwareName + "/" + s.ProducerName
}
