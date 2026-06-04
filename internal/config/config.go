package config

import (
	"fmt"
	"os"
)

// Config is the application configuration assembled by Load.
type Config struct {
	Software       SoftwareIdentity
	SigningKeyFile string
}

// Load merges the optional .env file at envPath into the process environment
// (real env vars win — see loadDotEnv), then builds and validates the
// configuration. Validation failures here fail the boot, so an identity AT
// would reject never reaches SAF-T emission.
func Load(envPath string) (Config, error) {
	if err := loadDotEnv(envPath); err != nil {
		return Config{}, fmt.Errorf("load %s: %w", envPath, err)
	}
	cfg := Config{
		Software: SoftwareIdentity{
			ProducerTaxID:     os.Getenv("PRODUCER_TAX_ID"),
			SoftwareName:      os.Getenv("SOFTWARE_NAME"),
			ProducerName:      os.Getenv("PRODUCER_NAME"),
			Version:           os.Getenv("VERSION"),
			CertificateNumber: os.Getenv("CERTIFICATE_NUMBER"),
		},
	}
	cfg.SigningKeyFile = os.Getenv("AT_SIGNING_KEY_FILE")
	if err := cfg.Software.Validate(); err != nil {
		return Config{}, fmt.Errorf("software identity (from %s + environment): %w", envPath, err)
	}
	return cfg, nil
}
