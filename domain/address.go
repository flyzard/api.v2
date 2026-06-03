package domain

import (
	"fmt"
	"regexp"
)

type Country string

var validCountries = map[Country]struct{}{
	"AD": {}, "AE": {}, "AF": {}, "AG": {}, "AI": {}, "AL": {}, "AM": {}, "AO": {},
	"AQ": {}, "AR": {}, "AS": {}, "AT": {}, "AU": {}, "AW": {}, "AX": {}, "AZ": {},
	"BA": {}, "BB": {}, "BD": {}, "BE": {}, "BF": {}, "BG": {}, "BH": {}, "BI": {},
	"BJ": {}, "BL": {}, "BM": {}, "BN": {}, "BO": {}, "BQ": {}, "BR": {}, "BS": {},
	"BT": {}, "BV": {}, "BW": {}, "BY": {}, "BZ": {}, "CA": {}, "CC": {}, "CD": {},
	"CF": {}, "CG": {}, "CH": {}, "CI": {}, "CK": {}, "CL": {}, "CM": {}, "CN": {},
	"CO": {}, "CR": {}, "CU": {}, "CV": {}, "CW": {}, "CX": {}, "CY": {}, "CZ": {},
	"DE": {}, "DJ": {}, "DK": {}, "DM": {}, "DO": {}, "DZ": {}, "EC": {}, "EE": {},
	"EG": {}, "EH": {}, "ER": {}, "ES": {}, "ET": {}, "FI": {}, "FJ": {}, "FK": {},
	"FM": {}, "FO": {}, "FR": {}, "GA": {}, "GB": {}, "GD": {}, "GE": {}, "GF": {},
	"GG": {}, "GH": {}, "GI": {}, "GL": {}, "GM": {}, "GN": {}, "GP": {}, "GQ": {},
	"GR": {}, "GS": {}, "GT": {}, "GU": {}, "GW": {}, "GY": {}, "HK": {}, "HM": {},
	"HN": {}, "HR": {}, "HT": {}, "HU": {}, "ID": {}, "IE": {}, "IL": {}, "IM": {},
	"IN": {}, "IO": {}, "IQ": {}, "IR": {}, "IS": {}, "IT": {}, "JE": {}, "JM": {},
	"JO": {}, "JP": {}, "KE": {}, "KG": {}, "KH": {}, "KI": {}, "KM": {}, "KN": {},
	"KP": {}, "KR": {}, "KW": {}, "KY": {}, "KZ": {}, "LA": {}, "LB": {}, "LC": {},
	"LI": {}, "LK": {}, "LR": {}, "LS": {}, "LT": {}, "LU": {}, "LV": {}, "LY": {},
	"MA": {}, "MC": {}, "MD": {}, "ME": {}, "MF": {}, "MG": {}, "MH": {}, "MK": {},
	"ML": {}, "MM": {}, "MN": {}, "MO": {}, "MP": {}, "MQ": {}, "MR": {}, "MS": {},
	"MT": {}, "MU": {}, "MV": {}, "MW": {}, "MX": {}, "MY": {}, "MZ": {}, "NA": {},
	"NC": {}, "NE": {}, "NF": {}, "NG": {}, "NI": {}, "NL": {}, "NO": {}, "NP": {},
	"NR": {}, "NU": {}, "NZ": {}, "OM": {}, "PA": {}, "PE": {}, "PF": {}, "PG": {},
	"PH": {}, "PK": {}, "PL": {}, "PM": {}, "PN": {}, "PR": {}, "PS": {}, "PT": {},
	"PW": {}, "PY": {}, "QA": {}, "RE": {}, "RO": {}, "RS": {}, "RU": {}, "RW": {},
	"SA": {}, "SB": {}, "SC": {}, "SD": {}, "SE": {}, "SG": {}, "SH": {}, "SI": {},
	"SJ": {}, "SK": {}, "SL": {}, "SM": {}, "SN": {}, "SO": {}, "SR": {}, "SS": {},
	"ST": {}, "SV": {}, "SX": {}, "SY": {}, "SZ": {}, "TC": {}, "TD": {}, "TF": {},
	"TG": {}, "TH": {}, "TJ": {}, "TK": {}, "TL": {}, "TM": {}, "TN": {}, "TO": {},
	"TR": {}, "TT": {}, "TV": {}, "TW": {}, "TZ": {}, "UA": {}, "UG": {}, "UM": {},
	"US": {}, "UY": {}, "UZ": {}, "VA": {}, "VC": {}, "VE": {}, "VG": {}, "VI": {},
	"VN": {}, "VU": {}, "WF": {}, "WS": {}, "XK": {}, "YE": {}, "YT": {}, "ZA": {},
	"ZM": {}, "ZW": {}, "Desconhecido": {},
}

func (c Country) IsValid() bool {
	_, ok := validCountries[c]
	return ok
}

func NewCountry(s string) (Country, error) {
	c := Country(s)
	if !c.IsValid() {
		return "", ErrInvalidCountry
	}
	return c, nil
}

func (c *Country) UnmarshalJSON(data []byte) error { return unmarshalString(data, NewCountry, c) }

type Address struct {
	BuildingNumber string  `json:"building_number,omitempty"`
	StreetName     string  `json:"street_name,omitempty"`
	AddressDetail  string  `json:"address_detail"`
	City           string  `json:"city"`
	PostalCode     string  `json:"postal_code"`
	Region         string  `json:"region,omitempty"`
	Country        Country `json:"country"`
}

var ptPostalCode = regexp.MustCompile(`^\d{4}-\d{3}$`)

func (a Address) Validate() error {
	if a.AddressDetail == "" {
		return ErrMissingAddressDetail
	}
	if a.City == "" {
		return ErrMissingCity
	}
	if a.PostalCode == "" {
		return ErrMissingPostalCode
	}
	if !a.Country.IsValid() {
		return ErrInvalidCountry
	}
	if a.Country == "PT" && !ptPostalCode.MatchString(a.PostalCode) {
		return fmt.Errorf("PT postal code must match NNNN-NNN: %q", a.PostalCode)
	}
	// BuildingNumber/StreetName/Region are length-unconstrained at the domain
	// layer (no entry in regras.md); revisit if the SAF-T projector pins
	// authoritative XSD lengths for them.
	for _, f := range []struct {
		name string
		val  string
		max  int
	}{
		{"address_detail", a.AddressDetail, MaxLenAddressDetail},
		{"city", a.City, MaxLenCity},
		{"postal_code", a.PostalCode, MaxLenPostalCode},
	} {
		if len(f.val) > f.max {
			return fmt.Errorf("%s exceeds %d chars: %q", f.name, f.max, f.val)
		}
	}
	return nil
}

func NewAddress(addressDetail, city, postalCode string, country Country) (Address, error) {
	a := Address{
		AddressDetail: addressDetail,
		City:          city,
		PostalCode:    postalCode,
		Country:       country,
	}
	return a, a.Validate()
}
