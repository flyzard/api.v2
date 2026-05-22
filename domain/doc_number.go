package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// docNumberPattern mirrors the XSD InvoiceNo/DocumentNumber/PaymentRefNo regex.
// Form: "{Type} {Series}/{Seq}", e.g. "FT A/1".
var docNumberPattern = regexp.MustCompile(`^[^ ]+ [^/^ ]+/[0-9]+$`)

// DocNumber is the composite identifier printed on issued documents.
type DocNumber struct {
	Type   DocumentType `json:"type"`
	Series string       `json:"series"`
	Seq    int          `json:"seq"`
}

func NewDocNumber(t DocumentType, series string, seq int) (DocNumber, error) {
	d := DocNumber{Type: t, Series: series, Seq: seq}
	return d, d.Validate()
}

func (d DocNumber) Validate() error {
	if !d.Type.IsValid() {
		return fmt.Errorf("invalid document type: %q", d.Type)
	}
	if d.Seq < 1 {
		return fmt.Errorf("seq must be >= 1: %d", d.Seq)
	}
	if err := ValidateSeries(d.Series); err != nil {
		return err
	}
	formatted := d.Format()
	if len(formatted) > MaxLenDocNumber {
		return fmt.Errorf("doc number exceeds %d chars: %q", MaxLenDocNumber, formatted)
	}
	if !docNumberPattern.MatchString(formatted) {
		return fmt.Errorf("doc number does not match SAF-T pattern: %q", formatted)
	}
	return nil
}

func (d DocNumber) Format() string {
	return fmt.Sprintf("%s %s/%d", d.Type, d.Series, d.Seq)
}

// ParseDocNumber inverts Format. Used by cross-document validation
// (e.g. ND product-set checks) where references are stored as the formatted
// "FT A/1" string and must be looked up via IssuedDocumentReader.
func ParseDocNumber(s string) (DocNumber, error) {
	space := strings.IndexByte(s, ' ')
	slash := strings.LastIndexByte(s, '/')
	if space < 0 || slash < 0 || slash <= space+1 {
		return DocNumber{}, fmt.Errorf("malformed doc number: %q", s)
	}
	seq, err := strconv.Atoi(s[slash+1:])
	if err != nil {
		return DocNumber{}, fmt.Errorf("malformed doc number seq: %q", s)
	}
	return NewDocNumber(DocumentType(s[:space]), s[space+1:slash], seq)
}
