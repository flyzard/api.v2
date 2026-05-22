package domain

import (
	"fmt"
	"strconv"
)

// ATCUD is the Unique Document Code (Código Único do Documento) mandated by Portaria 195/2020.
// Format: "<ValidationCode>-<Seq>". Max length 100 chars per XSD.
// Issuance gates require the series to be registered (CanIssue), so a fallback
// to "0" for unregistered series is unreachable and was removed (AUDIT 3.14).
type ATCUD string

func NewATCUD(series Series, seq int) (ATCUD, error) {
	if seq < 1 {
		return "", fmt.Errorf("atcud seq must be >= 1: %d", seq)
	}
	if !series.IsRegistered() {
		return "", fmt.Errorf("series %q is not registered with AT", series.ID)
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

// ValidateATCode checks the AT validation code returned by the WDT webservice.
// TODO(atcud-alphabet): the strict alphabet is [CONFIRMAR]; current rule is
// permissive (length >= 8, uppercase [A-Z0-9]) per FIX_PLAN.md §0.5 fallback.
// Tighten to AT's documented alphabet once the authoritative reference is located.
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
