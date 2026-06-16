package domain

import (
	"fmt"
	"strconv"
)

// ATCUD is the Unique Document Code (Código Único do Documento) mandated by Portaria 195/2020.
type ATCUD string

const MaxLenATCUD = 100

func NewATCUD(series Series, seq int) (ATCUD, error) {
	if seq < 1 {
		return "", fmt.Errorf("atcud seq must be >= 1: %d", seq)
	}
	if !series.IsRegistered() {
		return "", fmt.Errorf("series %q is not registered with AT", series.ID)
	}
	if err := ValidateATCode(series.ATCode); err != nil {
		return "", fmt.Errorf("series %q has invalid AT code: %w", series.ID, err)
	}
	atcud := ATCUD(series.ATCode + "-" + strconv.Itoa(seq))
	if len(atcud) > MaxLenATCUD {
		return "", fmt.Errorf("atcud exceeds %d chars: %q", MaxLenATCUD, atcud)
	}
	return atcud, nil
}

func (a ATCUD) Validate() error {
	if a == "" {
		return fmt.Errorf("atcud is required")
	}
	if len(a) > MaxLenATCUD {
		return fmt.Errorf("atcud exceeds %d chars: %q", MaxLenATCUD, a)
	}
	return nil
}

// ValidateATCode checks the AT validation code returned by the SeriesWS
// webservice (codValidacaoSerie — minLength 8 per the AT manual "Comunicação
// de Séries Documentais, Aspetos Específicos" v1.2, no maximum specified).
func ValidateATCode(code string) error {
	if len(code) < 8 {
		return fmt.Errorf("at code length must be >= 8, got %d (%q)", len(code), code)
	}
	for _, c := range []byte(code) {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return fmt.Errorf("at code must be uppercase [A-Z0-9]: %q", code)
		}
	}
	return nil
}
