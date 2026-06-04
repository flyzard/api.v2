package domain

import "strings"

type TaxID string

func NewTaxID(s string) (TaxID, error) {
	t := TaxID(strings.TrimSpace(s))
	if !t.IsValid() {
		return "", ErrInvalidTaxID
	}
	return t, nil
}

func (t *TaxID) UnmarshalJSON(data []byte) error { return unmarshalString(data, NewTaxID, t) }

var nifPrefixes2 = map[string]struct{}{
	"45": {}, "70": {}, "71": {}, "72": {}, "74": {}, "75": {},
	"77": {}, "79": {}, "90": {}, "91": {}, "98": {}, "99": {},
}

func (t TaxID) IsValid() bool {
	s := strings.TrimSpace(string(t))
	if len(s) != 9 {
		return false
	}
	for i := range 9 {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	if !validNIFPrefix(s) {
		return false
	}
	sum := 0
	for i := range 8 {
		sum += int(s[i]-'0') * (9 - i)
	}
	check := 11 - sum%11
	if check >= 10 {
		check = 0
	}
	return check == int(s[8]-'0')
}

func validNIFPrefix(s string) bool {
	switch s[0] {
	case '1', '2', '3', '5', '6', '8':
		return true
	}
	_, ok := nifPrefixes2[s[:2]]
	return ok
}
