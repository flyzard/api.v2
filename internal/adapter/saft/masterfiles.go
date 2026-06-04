package saft

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"github.com/google/uuid"
)

// xmlMasterFiles mirrors SAF-T AuditFile/MasterFiles. For TaxAccountingBasis="F"
// the only required children are Customer, Product, and TaxTable.
type xmlMasterFiles struct {
	Customers []xmlCustomer `xml:"Customer"`
	Products  []xmlProduct  `xml:"Product"`
	TaxTable  xmlTaxTable   `xml:"TaxTable"`
}

type xmlCustomer struct {
	CustomerID           string     `xml:"CustomerID"`
	AccountID            string     `xml:"AccountID"`
	CustomerTaxID        string     `xml:"CustomerTaxID"`
	CompanyName          string     `xml:"CompanyName"`
	Contact              string     `xml:"Contact,omitempty"`
	BillingAddress       xmlAddress `xml:"BillingAddress"`
	Telephone            string     `xml:"Telephone,omitempty"`
	Fax                  string     `xml:"Fax,omitempty"`
	Email                string     `xml:"Email,omitempty"`
	Website              string     `xml:"Website,omitempty"`
	SelfBillingIndicator int        `xml:"SelfBillingIndicator"`
}

type xmlProduct struct {
	ProductType        string `xml:"ProductType"`
	ProductCode        string `xml:"ProductCode"`
	ProductGroup       string `xml:"ProductGroup,omitempty"`
	ProductDescription string `xml:"ProductDescription"`
	ProductNumberCode  string `xml:"ProductNumberCode"`
}

type xmlTaxTable struct {
	Entries []xmlTaxEntry `xml:"TaxTableEntry"`
}

type xmlTaxEntry struct {
	TaxType          string `xml:"TaxType"`
	TaxCountryRegion string `xml:"TaxCountryRegion"`
	TaxCode          string `xml:"TaxCode"`
	Description      string `xml:"Description"`
	TaxPercentage    string `xml:"TaxPercentage,omitempty"`
	TaxAmount        string `xml:"TaxAmount,omitempty"`
}

// buildMasterFiles dedups Customers/Products/TaxTable across every issued
// document. Drift on a shared key returns an error (AT cert §5.6/§5.10).
// TaxTable percentages come from observed line VAT rates so per-line and
// per-table values always reconcile.
func buildMasterFiles(sales []domain.SalesInvoice, stock []domain.StockMovement,
	work []domain.WorkDocument, payments []domain.Payment) (xmlMasterFiles, error) {

	custs := map[uuid.UUID]xmlCustomer{}
	prods := map[string]xmlProduct{}
	rates := map[taxKey]domain.Percent{}

	addCust := func(c domain.Customer) error {
		next := buildCustomer(c)
		if prev, ok := custs[c.CustomerID]; ok && prev != next {
			return fmt.Errorf("customer %s has drift: %q vs %q", c.CustomerID, prev.CompanyName, next.CompanyName)
		}
		custs[c.CustomerID] = next
		return nil
	}
	addLines := func(lines []domain.DocumentLine) error {
		for _, l := range lines {
			next := buildProduct(l.Product)
			if prev, ok := prods[next.ProductCode]; ok && prev != next {
				return fmt.Errorf("product %q has drift in MasterFiles vs line: description %q vs %q", next.ProductCode, prev.ProductDescription, next.ProductDescription)
			}
			prods[next.ProductCode] = next
			if vat, ok := l.Tax.(domain.VATTax); ok {
				k := taxKey{vat.Rate.Region, vat.Rate.Category}
				if prev, seen := rates[k]; seen && prev != vat.Rate.Value {
					return fmt.Errorf("tax rate drift for %s/%s: %d bp vs %d bp", k.Region, k.Category, prev, vat.Rate.Value)
				}
				rates[k] = vat.Rate.Value
			}
		}
		return nil
	}

	walk := func(c domain.Customer, lines []domain.DocumentLine) error {
		if err := addCust(c); err != nil {
			return err
		}
		return addLines(lines)
	}
	for _, d := range sales {
		if err := walk(d.Customer, d.Lines); err != nil {
			return xmlMasterFiles{}, err
		}
	}
	for _, d := range stock {
		if err := walk(d.Customer, d.Lines); err != nil {
			return xmlMasterFiles{}, err
		}
	}
	for _, d := range work {
		if err := walk(d.Customer, d.Lines); err != nil {
			return xmlMasterFiles{}, err
		}
	}
	for _, d := range payments {
		if err := addCust(d.Customer); err != nil {
			return xmlMasterFiles{}, err
		}
	}

	return xmlMasterFiles{
		Customers: sortedValues(custs, func(c xmlCustomer) string { return c.AccountID }),
		Products:  sortedValues(prods, func(p xmlProduct) string { return p.ProductCode }),
		TaxTable:  buildTaxTable(rates),
	}, nil
}

func buildCustomer(c domain.Customer) xmlCustomer {
	return xmlCustomer{
		CustomerID:           c.CustomerID.String(),
		AccountID:            c.AccountID,
		CustomerTaxID:        string(c.CustomerTaxID),
		CompanyName:          c.CompanyName,
		Contact:              c.Contact,
		BillingAddress:       buildAddress(c.BillingAddress),
		Telephone:            c.Telephone,
		Fax:                  c.Fax,
		Email:                c.Email,
		Website:              c.Website,
		SelfBillingIndicator: boolToInt(c.SelfBillingIndicator),
	}
}

func buildProduct(p domain.Product) xmlProduct {
	return xmlProduct{
		ProductType:        string(p.ProductType),
		ProductCode:        p.ProductCode,
		ProductGroup:       p.ProductGroup,
		ProductDescription: p.ProductDescription,
		ProductNumberCode:  p.ProductNumberCode,
	}
}

type taxKey struct {
	Region   domain.TaxRegion
	Category domain.TaxCategory
}

// Canonical descriptions for the VAT category codes that appear in TaxTable.
var taxCategoryDescription = map[domain.TaxCategory]string{
	domain.TaxNormal:       "Taxa Normal",
	domain.TaxIntermediate: "Taxa Intermédia",
	domain.TaxReduced:      "Taxa Reduzida",
	domain.TaxExempt:       "Isento",
	domain.TaxOther:        "Outra",
}

func buildTaxTable(rates map[taxKey]domain.Percent) xmlTaxTable {
	out := make([]xmlTaxEntry, 0, len(rates))
	for k, pct := range rates {
		out = append(out, xmlTaxEntry{
			TaxType:          "IVA",
			TaxCountryRegion: string(k.Region),
			TaxCode:          string(k.Category),
			Description:      taxCategoryDescription[k.Category],
			TaxPercentage:    fmtPercent(pct),
		})
	}
	slices.SortFunc(out, func(a, b xmlTaxEntry) int {
		return cmp.Or(
			cmp.Compare(a.TaxCountryRegion, b.TaxCountryRegion),
			cmp.Compare(a.TaxCode, b.TaxCode),
		)
	})
	return xmlTaxTable{Entries: out}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
