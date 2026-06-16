package config

import "testing"

func validIdentity() SoftwareIdentity {
	return SoftwareIdentity{
		ProducerTaxID:     "519348761", // valid PT NIF (mod-11) — used in saft golden export_test.go
		SoftwareName:      "Faturly",
		ProducerName:      "Avenida do Código Lda",
		Version:           "1.0.0",
		CertificateNumber: "9999",
	}
}

func TestSoftwareIdentity_Validate(t *testing.T) {
	if err := validIdentity().Validate(); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*SoftwareIdentity)
		wantOK bool
	}{
		{"cert numeric 0", func(s *SoftwareIdentity) { s.CertificateNumber = "0" }, true},
		{"cert non-numeric", func(s *SoftwareIdentity) { s.CertificateNumber = "ABC" }, false},
		{"cert decimal", func(s *SoftwareIdentity) { s.CertificateNumber = "9.9" }, false},
		{"cert signed", func(s *SoftwareIdentity) { s.CertificateNumber = "+9" }, false},
		{"cert negative", func(s *SoftwareIdentity) { s.CertificateNumber = "-1" }, false},
		{"cert empty", func(s *SoftwareIdentity) { s.CertificateNumber = "" }, false},
		{"cert too long", func(s *SoftwareIdentity) { s.CertificateNumber = "12345678901" }, false},
		{"name has slash", func(s *SoftwareIdentity) { s.SoftwareName = "Fat/urly" }, false},
		{"producer has slash", func(s *SoftwareIdentity) { s.ProducerName = "A/B Lda" }, false},
		{"bad nif", func(s *SoftwareIdentity) { s.ProducerTaxID = "500000001" }, false}, // fails PT mod-11 checksum (check digit must be 0, not 1)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := validIdentity()
			c.mutate(&s)
			err := s.Validate()
			if c.wantOK && err != nil {
				t.Errorf("want ok, got %v", err)
			}
			if !c.wantOK && err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
