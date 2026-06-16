package domain

import (
	"encoding/json"
	"fmt"
)

type TaxRegion string

const (
	PT   TaxRegion = "PT"
	PTAC TaxRegion = "PT-AC"
	PTMA TaxRegion = "PT-MA"
)

type TaxCategory string

const (
	TaxNormal       TaxCategory = "NOR"
	TaxIntermediate TaxCategory = "INT"
	TaxReduced      TaxCategory = "RED"
	TaxExempt       TaxCategory = "ISE"
	// TaxOther covers "other" VAT codes that don't map to the canonical
	// NOR/INT/RED rate table — the line declares its own rate (e.g. parafiscal
	// pass-throughs). GetTaxRate cannot resolve OUT; build TaxRate directly.
	TaxOther TaxCategory = "OUT"
)

// RegionRates holds VAT rates per category in Percent units (basis points):
// 2300 = 23.00%, 650 = 6.50%.
type RegionRates struct {
	Normal, Intermediate, Reduced Percent
}

// taxRates is hardcoded and trusted. If rates ever come from an external source,
// validate values through NewPercent before assigning.
var taxRates = map[TaxRegion]RegionRates{
	PT:   {Normal: 2300, Intermediate: 1300, Reduced: 600},
	PTAC: {Normal: 1600, Intermediate: 900, Reduced: 400},
	PTMA: {Normal: 2200, Intermediate: 1200, Reduced: 500},
}

func (r RegionRates) rateFor(category TaxCategory) (Percent, error) {
	switch category {
	case TaxNormal:
		return r.Normal, nil
	case TaxIntermediate:
		return r.Intermediate, nil
	case TaxReduced:
		return r.Reduced, nil
	default:
		return 0, fmt.Errorf("unknown tax category: %s", category)
	}
}

// TaxRate: Category ISE iff valid Exemption (enforced by GetTaxRate).
type TaxRate struct {
	Region    TaxRegion   `json:"region"`
	Category  TaxCategory `json:"category"`
	Value     Percent     `json:"value"`
	Exemption Exemption   `json:"exemption,omitempty"`
}

// UnmarshalJSON re-derives Value from the canonical rate table for the standard
// categories. For TaxOther — which has no canonical rate — the client-supplied
// Value is preserved.
func (t *TaxRate) UnmarshalJSON(data []byte) error {
	var in struct {
		Region    TaxRegion   `json:"region"`
		Category  TaxCategory `json:"category"`
		Value     Percent     `json:"value"`
		Exemption Exemption   `json:"exemption"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	if in.Category == TaxOther {
		r := TaxRate{Region: in.Region, Category: in.Category, Value: in.Value, Exemption: in.Exemption}
		if err := r.Validate(); err != nil {
			return err
		}
		*t = r
		return nil
	}
	rate, err := GetTaxRate(in.Region, in.Category, in.Exemption)
	if err != nil {
		return err
	}
	*t = rate
	return nil
}

// Validate asserts the rate matches the canonical table. JSON path canonicalizes
// via UnmarshalJSON; this catches programmatic literals like TaxRate{...}.
// For TaxOther the canonical lookup is skipped — the line declares its own rate;
// only the region and a non-negative value are checked.
func (t TaxRate) Validate() error {
	if t.Category == TaxOther {
		if _, ok := taxRates[t.Region]; !ok {
			return fmt.Errorf("unknown tax region: %s", t.Region)
		}
		if t.Value < 0 || t.Value > PercentScale {
			return fmt.Errorf("OUT rate value out of range: %d", t.Value)
		}
		return nil
	}
	expected, err := GetTaxRate(t.Region, t.Category, t.Exemption)
	if err != nil {
		return err
	}
	if expected.Value != t.Value {
		return fmt.Errorf("tax rate value %d does not match canonical %d for %s/%s", t.Value, expected.Value, t.Region, t.Category)
	}
	return nil
}

func GetTaxRate(region TaxRegion, category TaxCategory, exemption Exemption) (TaxRate, error) {
	rates, ok := taxRates[region]
	if !ok {
		return TaxRate{}, fmt.Errorf("unknown tax region: %s", region)
	}
	if category == TaxOther {
		return TaxRate{}, fmt.Errorf("category %s has no canonical rate; construct TaxRate directly with caller-supplied Value", TaxOther)
	}
	if category == TaxExempt {
		// Policy (Despacho 8632/2014 §3.2.6): the issuer must state the exemption
		// reason — never silently default to M99. M99 is accepted only when the
		// caller passes it explicitly as the catch-all.
		if !exemption.Valid() {
			return TaxRate{}, fmt.Errorf("category %s requires a valid exemption, got %q", TaxExempt, exemption)
		}
		return TaxRate{Region: region, Category: category, Exemption: exemption}, nil
	}
	if exemption != "" {
		return TaxRate{}, fmt.Errorf("exemption %s requires category %s, got %s", exemption, TaxExempt, category)
	}
	value, err := rates.rateFor(category)
	if err != nil {
		return TaxRate{}, err
	}
	return TaxRate{Region: region, Category: category, Value: value}, nil
}

type Exemption string

const (
	M01 Exemption = "M01" // Art. 16.º n.º 6 CIVA - reimbursable expenses
	M02 Exemption = "M02" // M02 is an exemption for sales to national exporters per DL 198/90 Art. 6.o.
	M04 Exemption = "M04" // M04 is an exemption for exempt imports per Art. 13.o CIVA.
	M05 Exemption = "M05" // M05 is an exemption for exports and international transport per Art. 14.o CIVA.
	M06 Exemption = "M06" // M06 is an exemption for suspensive customs regimes per Art. 15.o CIVA.
	M07 Exemption = "M07" // M07 is an exemption for health, education, and social services per Art. 9.o CIVA.
	M09 Exemption = "M09" // M09 is an exemption for small retailers regime per Art. 62.o b) CIVA.
	M10 Exemption = "M10" // M10 is an exemption for small business per Art. 53.o CIVA.
	M11 Exemption = "M11" // M11 is an exemption for the tobacco special regime per DL 346/85.
	M12 Exemption = "M12" // M12 is an exemption for travel agencies margin scheme per DL 221/85.
	M13 Exemption = "M13" // M13 is an exemption for second-hand goods margin scheme per DL 199/96.
	M14 Exemption = "M14" // M14 is an exemption for art objects margin scheme per DL 199/96.
	M15 Exemption = "M15" // M15 is an exemption for collectibles and antiques margin scheme per DL 199/96.
	M16 Exemption = "M16" // M16 is an exemption for intra-community supplies per Art. 14.o RITI.
	M19 Exemption = "M19" // M19 is an exemption for temporary exemptions determined by specific diploma.
	M20 Exemption = "M20" // M20 is an exemption for flat-rate farmers regime per Art. 59.o-D n.o 2 CIVA.
	M21 Exemption = "M21" // M21 is an exemption for resellers/distributors regime per Art. 72.o n.o 4 CIVA.
	M25 Exemption = "M25" // M25 is an exemption for goods on consignment per Art. 38.o n.o 1 a) CIVA.
	M26 Exemption = "M26" // M26 is a zero-rate (with deduction) exemption for the basic food basket per Lei n.º 17/2023.
	M30 Exemption = "M30" // M30 is a reverse charge code for waste/scrap/recyclables per Art. 2.o n.o 1 i) CIVA.
	M31 Exemption = "M31" // M31 is a reverse charge code for construction services per Art. 2.o n.o 1 j) CIVA.
	M32 Exemption = "M32" // M32 is a reverse charge code for greenhouse gas emissions per Art. 2.o n.o 1 l) CIVA.
	M33 Exemption = "M33" // M33 is a reverse charge code for cork, wood, and pinecones per Art. 2.o n.o 1 m) CIVA.
	M34 Exemption = "M34" // M34 is a reverse charge code for self-consumption electricity per Art. 2.o n.o 1 n) CIVA.
	M40 Exemption = "M40" // M40 is a reverse charge code for services from non-residents per Art. 6.o n.o 6 a) CIVA.
	M41 Exemption = "M41" // M41 is a reverse charge code for triangular operations per Art. 8.o n.o 3 RITI.
	M42 Exemption = "M42" // M42 is a reverse charge code for real estate exemption waiver per DL 21/2007.
	M43 Exemption = "M43" // M43 is a reverse charge code for investment gold exemption waiver per DL 362/99.
	M44 Exemption = "M44" // M44 is for operations outside PT territory per Art. 6.o CIVA (2025 code).
	M45 Exemption = "M45" // M45 is for cross-border exemption regime per Art. 58.o-A CIVA (2025 code).
	M46 Exemption = "M46" // M46 is for e-TaxFree tourist VAT refunds per DL 19/2017 (2025 code).
	M99 Exemption = "M99" // M99 is the catch-all exemption code per Art. 2.o n.o 2, 3.o n.os 4/6/7, 4.o n.o 5 CIVA.
)

// exemptionInfo holds the per-code metadata that lookups (Valid/Description/IsReverseCharge)
// need. One row per code = one source of truth for what a code means and how it behaves.
type exemptionInfo struct {
	Description     string
	IsReverseCharge bool
}

var exemptions = map[Exemption]exemptionInfo{
	// Exemptions
	M01: {Description: "Artigo 16.º, n.º 6 do CIVA"},
	M02: {Description: "Artigo 6.º do Decreto-Lei n.º 198/90, de 19 de junho"},
	M04: {Description: "Isento artigo 13.º do CIVA"},
	M05: {Description: "Isento artigo 14.º do CIVA"},
	M06: {Description: "Isento artigo 15.º do CIVA"},
	M07: {Description: "Isento artigo 9.º do CIVA"},
	M09: {Description: "IVA - não confere direito a dedução / Artigo 62.º alínea b) do CIVA"},
	M10: {Description: "IVA - Regime de isenção / Artigo 53.º do CIVA"},
	M11: {Description: "Regime particular do tabaco / Decreto-Lei n.º 346/85, de 23 de agosto"},
	M12: {Description: "Regime da margem de lucro - Agências de viagens / Decreto-Lei n.º 221/85, de 3 de julho"},
	M13: {Description: "Regime da margem de lucro - Bens em segunda mão / Decreto-Lei n.º 199/96, de 18 de outubro"},
	M14: {Description: "Regime da margem de lucro - Objetos de arte / Decreto-Lei n.º 199/96, de 18 de outubro"},
	M15: {Description: "Regime da margem de lucro - Objetos de coleção e antiguidades / Decreto-Lei n.º 199/96, de 18 de outubro"},
	M16: {Description: "Isento artigo 14.º do RITI"},
	M19: {Description: "Outras isenções temporárias determinadas em diploma próprio"},
	M20: {Description: "IVA - regime forfetário / Artigo 59.º-D n.º 2 do CIVA"},
	M21: {Description: "IVA - não confere direito a dedução / Artigo 72.º n.º 4 do CIVA"},
	M25: {Description: "Mercadorias à consignação / Artigo 38.º n.º 1 alínea a) do CIVA"},
	M26: {Description: "Isenção de IVA com direito à dedução no cabaz alimentar / Lei n.º 17/2023, de 14 de abril"},

	// Reverse charge (autoliquidação)
	M30: {Description: "IVA - autoliquidação / Artigo 2.º n.º 1 alínea i) do CIVA", IsReverseCharge: true},
	M31: {Description: "IVA - autoliquidação / Artigo 2.º n.º 1 alínea j) do CIVA", IsReverseCharge: true},
	M32: {Description: "IVA - autoliquidação / Artigo 2.º n.º 1 alínea l) do CIVA", IsReverseCharge: true},
	M33: {Description: "IVA - autoliquidação / Artigo 2.º n.º 1 alínea m) do CIVA", IsReverseCharge: true},
	M34: {Description: "IVA - autoliquidação / Artigo 2.º n.º 1 alínea n) do CIVA", IsReverseCharge: true},
	M40: {Description: "IVA - autoliquidação / Artigo 6.º n.º 6 alínea a) do CIVA, a contrário", IsReverseCharge: true},
	M41: {Description: "IVA - autoliquidação / Artigo 8.º n.º 3 do RITI", IsReverseCharge: true},
	M42: {Description: "IVA - autoliquidação / Decreto-Lei n.º 21/2007, de 29 de janeiro", IsReverseCharge: true},
	M43: {Description: "IVA - autoliquidação / Decreto-Lei n.º 362/99, de 16 de setembro", IsReverseCharge: true},

	// 2025 codes
	M44: {Description: "Artigo 6.º do CIVA - operações não localizadas em território nacional"},
	M45: {Description: "Artigo 58.º-A do CIVA - regime de isenção transfronteiriço"},
	M46: {Description: "Decreto-Lei n.º 19/2017, de 14 de fevereiro - e-TaxFree"},

	// Catch-all
	M99: {Description: "Não sujeito ou não tributado"},
}

func (e Exemption) Valid() bool {
	_, ok := exemptions[e]
	return ok
}

func (e Exemption) Description() string {
	if info, ok := exemptions[e]; ok {
		return info.Description
	}
	return string(e)
}

func (e Exemption) IsReverseCharge() bool { return exemptions[e].IsReverseCharge }

// euMemberStates is the EU-27 in ISO 3166-1 alpha-2 (Greece is "GR" in ISO
// even though VIES VAT prefixes use "EL"). Consumed by the M16 issuance gate:
// RITI Art. 14.º n.º 1 a) exempts intra-community supplies only when the buyer
// is VAT-registered in another Member State.
var euMemberStates = map[Country]bool{
	"AT": true, "BE": true, "BG": true, "HR": true, "CY": true, "CZ": true,
	"DK": true, "EE": true, "FI": true, "FR": true, "DE": true, "GR": true,
	"HU": true, "IE": true, "IT": true, "LV": true, "LT": true, "LU": true,
	"MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SK": true,
	"SI": true, "ES": true, "SE": true,
}

// TaxBreakdownEntry is one row of the per-document aggregation grouped by
// (Region, Category, ExemptionCode). The SAF-T projector and the QR fields
// (I/J/K series) both consume this shape; computing it once at issuance keeps
// the projector pure.
type TaxBreakdownEntry struct {
	Region               TaxRegion   `json:"region"`
	Category             TaxCategory `json:"category"`
	ExemptionCode        Exemption   `json:"exemption_code,omitempty"`
	ExemptionDescription string      `json:"exemption_description,omitempty"`
	Base                 Money       `json:"base"`
	Tax                  Money       `json:"tax"`
}

// TaxBreakdown is a deterministically ordered slice (region asc, category asc,
// exemption code asc) so two documents with identical totals always produce
// identical breakdowns.
type TaxBreakdown []TaxBreakdownEntry

type taxBreakdownKey struct {
	Region    TaxRegion
	Category  TaxCategory
	Exemption Exemption
}
