// AT (Autoridade Tributária) requires textual fields in SAF-T tax filings to be
// representable in Windows-1252 (Portaria 363/2010 §R-G7). That is a regulatory
// invariant on the domain — every value object's text fields are pre-validated
// at construction so non-conformant invoices are unrepresentable in the model.
// The serialization-time check lives in saft/encode.go and is the byte-emission
// fallback for any path that bypasses VO constructors (raw struct literals,
// future ingress code).
package domain

import (
	"fmt"

	"golang.org/x/text/encoding/charmap"
)

// enforceWindows1252 returns an error if s contains any rune not representable
// in Windows-1252, naming the field and the first unmappable rune. Empty
// strings are accepted — required-field checks live elsewhere.
func enforceWindows1252(s, field string) error {
	for _, r := range s {
		if _, ok := charmap.Windows1252.EncodeRune(r); !ok {
			return fmt.Errorf("%s: rune %q (U+%04X) not representable in Windows-1252", field, r, r)
		}
	}
	return nil
}

// EnsureWindows1252 exposes the Windows-1252 charset invariant to non-domain
// validators (e.g. package config) that build SAF-T values from outside the
// model's value-object constructors.
func EnsureWindows1252(s, field string) error { return enforceWindows1252(s, field) }
