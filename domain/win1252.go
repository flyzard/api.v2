package domain

import (
	"fmt"

	"golang.org/x/text/encoding/charmap"
)

// enforceWindows1252 rejects strings containing any rune not representable in
// Windows-1252. AT mandates that encoding for the SAF-T XML export (regras §R-G7);
// catching the violation at construction keeps illegal state from ever being
// instantiated. Empty strings are accepted — required-field checks live elsewhere.
//
// The returned error names the field and the first unmappable rune.
func enforceWindows1252(s, field string) error {
	for _, r := range s {
		if _, ok := charmap.Windows1252.EncodeRune(r); !ok {
			return fmt.Errorf("%s: rune %q (U+%04X) not representable in Windows-1252", field, r, r)
		}
	}
	return nil
}
