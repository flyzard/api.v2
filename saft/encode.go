package saft

import (
	"fmt"

	"golang.org/x/text/encoding/charmap"
)

// SAF-T PT requires Windows-1252 (Portaria 363/2010, regras §R-G7). We emit
// our own XML declaration so the encoding attribute matches the actual byte
// representation — never use encoding/xml's xml.Header here (it hardcodes
// "UTF-8"). XSD validation out-of-band requires XSD 1.1 (Xerces-J / Saxon EE);
// xmllint can't compile the schema (uses xs:assert + unbounded maxOccurs).
const xmlDeclarationWin1252 = `<?xml version="1.0" encoding="Windows-1252"?>` + "\n"

// transcodeWin1252 converts a UTF-8 buffer to Windows-1252. The domain
// pre-validates each text field at VO construction (see domain/at_charset.go),
// so this step is the byte-emission fallback — an error here means a non-VO
// path (struct literal, future ingress) leaked an unmappable rune.
func transcodeWin1252(utf8 []byte) ([]byte, error) {
	out, err := charmap.Windows1252.NewEncoder().Bytes(utf8)
	if err != nil {
		return nil, fmt.Errorf("transcode UTF-8 → Windows-1252: %w", err)
	}
	return out, nil
}
