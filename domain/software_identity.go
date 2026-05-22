package domain

import "fmt"

// SoftwareIdentity identifies the certified software producer — NOT the issuing
// company. SAF-T Header's ProductCompanyTaxID and ProductID come from here; the
// QR R field carries CertificateNumber. AT certifies a specific (producer NIF,
// software name, version, certificate) tuple; a build that mutates any of these
// at runtime breaks the certification.
//
// Storage source (build-time const, config, DB) is Q-1 in FIX_PLAN.md §8.5 and
// remains open. The domain only models the value; production callers load it
// once at startup and pass it down to the SAF-T projector and QR generator.
type SoftwareIdentity struct {
	// ProducerTaxID is the certified producer's NIF (distinct from Company.NIF
	// which is the issuer's). E.g. "519348761" for AVENIDA DO CODIGO.
	ProducerTaxID TaxID `json:"producer_tax_id"`
	// SoftwareName is the AT-registered name (e.g. "Faturly").
	SoftwareName string `json:"software_name"`
	// ProducerName is the producer's legal name (e.g. "AVENIDA DO CODIGO LDA").
	ProducerName string `json:"producer_name"`
	// Version must match the Modelo 24 declaration on file with AT.
	Version string `json:"version"`
	// CertificateNumber is the AT-assigned numeric ID; appears in QR field R.
	CertificateNumber string `json:"certificate_number"`
}

func NewSoftwareIdentity(s SoftwareIdentity) (SoftwareIdentity, error) {
	if err := s.Validate(); err != nil {
		return SoftwareIdentity{}, err
	}
	return s, nil
}

func (s SoftwareIdentity) Validate() error {
	if !s.ProducerTaxID.IsValid() {
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
	for _, f := range []struct{ name, val string }{
		{"software_identity.software_name", s.SoftwareName},
		{"software_identity.producer_name", s.ProducerName},
		{"software_identity.version", s.Version},
	} {
		if err := enforceWindows1252(f.val, f.name); err != nil {
			return err
		}
	}
	return nil
}

// ProductID is the SAF-T Header.ProductID format — "SoftwareName/ProducerName".
// Projector emits this verbatim.
func (s SoftwareIdentity) ProductID() string {
	return s.SoftwareName + "/" + s.ProducerName
}
