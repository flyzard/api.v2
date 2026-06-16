package saft

import (
	"math"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// matrixCase is one labelled SAF-T export scenario exercising a tricky field
// combination the compliance audit flagged. It is the shared input for the XSD
// gate (xsd_test.go) and the reconciliation tests (reconcile_test.go).
type matrixCase struct {
	name     string
	sales    []domain.SalesInvoice
	stock    []domain.StockMovement
	work     []domain.WorkDocument
	payments []domain.Payment
}

// matrixDocs enumerates "the combinations AT will throw at us". It builds on the
// existing minimalSalesInvoice()/goldenDocs() fixtures (export_test.go,
// golden_test.go) and mutates the one field each case is about.
func matrixDocs() []matrixCase {
	date := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)

	// vat_exempt: a single exempt line (TaxExempt + M10 code + reason), tax 0.
	exempt := minimalSalesInvoice()
	exempt.Lines[0].Tax = must(domain.NewVATLineTax(domain.PT, domain.TaxExempt, domain.M10, "IVA - regime de isenção"))
	exempt.Totals = domain.Totals{
		NetTotal:   must(domain.NewMoney(100.00)),
		TaxTotal:   must(domain.NewMoney(0.00)),
		GrossTotal: must(domain.NewMoney(100.00)),
	}

	// fx_currency: foreign-currency block; Amount equals GrossTotal (the correct
	// M-Currency shape — 123.00 EUR gross expressed in the original currency).
	fx := minimalSalesInvoice()
	fx.Currency = &domain.Currency{
		Code:         "USD",
		Amount:       must(domain.NewMoney(123.00)),
		ExchangeRate: must(domain.NewExchangeRate(1.085)),
		Date:         date,
	}

	// nc_debit: a credit note (lines project to DebitAmount).
	nc := minimalSalesInvoice()
	nc.Number = must(domain.NewDocNumber(domain.NC, "NC2026", 1))
	nc.DocumentType = domain.NC
	nc.ATCUD = "AAAAAAAA-2"

	// recovered_M_form: SourceBilling=M + M-form HashControl.
	recM := minimalSalesInvoice()
	recM.Number = must(domain.NewDocNumber(domain.FT, "FTM2026", 1))
	recM.ATCUD = "AAAAAAAA-7"
	recM.SourceBilling = domain.SourceBillingManual
	recM.HashControl = "1-FTM M2025/7"

	// recovered_D_form: SourceBilling=M + D-form HashControl.
	recD := minimalSalesInvoice()
	recD.Number = must(domain.NewDocNumber(domain.FT, "FTD2026", 1))
	recD.ATCUD = "AAAAAAAA-8"
	recD.SourceBilling = domain.SourceBillingManual
	recD.HashControl = "1-FTD FT B2025/9"

	// taxbase_tax_only_line: UnitPrice 0 + TaxBase>0 (tax-only adjustment line).
	// The projector currently DROPS TaxBase — this case is expected to FAIL the
	// XSD gate until Phase 3 (M-TaxBase). It must still Export without a Go error.
	tb := minimalSalesInvoice()
	tb.Number = must(domain.NewDocNumber(domain.FT, "FTB2026", 1))
	tb.ATCUD = "AAAAAAAA-9"
	base := must(domain.NewMoney(100.00))
	tb.Lines[0].UnitPrice = must(domain.NewMoney(0))
	tb.Lines[0].TaxBase = &base

	// Reuse the golden GT/PF and RG/RG-cancelled fixtures for movement/work/payment coverage.
	_, stock, work, payments := goldenDocs()

	return []matrixCase{
		{"base_nor", []domain.SalesInvoice{minimalSalesInvoice()}, nil, nil, nil},
		{"vat_exempt", []domain.SalesInvoice{exempt}, nil, nil, nil},
		{"fx_currency", []domain.SalesInvoice{fx}, nil, nil, nil},
		{"nc_debit", []domain.SalesInvoice{nc}, nil, nil, nil},
		{"recovered_M_form", []domain.SalesInvoice{recM}, nil, nil, nil},
		{"recovered_D_form", []domain.SalesInvoice{recD}, nil, nil, nil},
		{"taxbase_tax_only_line", []domain.SalesInvoice{tb}, nil, nil, nil},
		{"rc_payment", nil, nil, nil, payments},
		{"stock_and_work", nil, stock, work, nil},
	}
}

// mustExport runs the projector for a matrix case and fails the test on a Go-level error.
func mustExport(t *testing.T, sales []domain.SalesInvoice, stock []domain.StockMovement, work []domain.WorkDocument, payments []domain.Payment) []byte {
	t.Helper()
	hdr := gdTestHeader()
	hdr.Issuer.EACCode = "47190"
	out, err := Export(hdr, sales, stock, work, payments)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return out
}

var docTotalRe = func() map[string]*regexp.Regexp {
	m := map[string]*regexp.Regexp{}
	for _, tag := range []string{"NetTotal", "TaxPayable", "GrossTotal"} {
		m[tag] = regexp.MustCompile("<" + tag + ">([0-9.]+)</" + tag + ">")
	}
	return m
}()

// parseDocTotals returns the FIRST DocumentTotals' Net/TaxPayable/Gross as integer cents.
func parseDocTotals(t *testing.T, xml []byte) (netCents, taxCents, grossCents int) {
	t.Helper()
	get := func(tag string) int {
		mm := docTotalRe[tag].FindSubmatch(xml)
		if mm == nil {
			t.Fatalf("no <%s> in export", tag)
		}
		f, err := strconv.ParseFloat(string(mm[1]), 64)
		if err != nil {
			t.Fatalf("parse <%s>=%q: %v", tag, mm[1], err)
		}
		return int(math.Round(f * 100))
	}
	return get("NetTotal"), get("TaxPayable"), get("GrossTotal")
}

func cents2dp(t *testing.T, s string) int {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return int(math.Round(f * 100))
}

// firstTagCents returns the FIRST <tag>…</tag> 2dp money value as integer cents.
func firstTagCents(t *testing.T, xml []byte, tag string) int {
	t.Helper()
	re := regexp.MustCompile("<" + tag + ">([0-9.]+)</" + tag + ">")
	mm := re.FindSubmatch(xml)
	if mm == nil {
		t.Fatalf("no <%s> in export", tag)
	}
	return cents2dp(t, string(mm[1]))
}

// TestMatrix_ExportsCleanly is a sanity gate: every matrix case must project
// without a Go-level error (XSD validity is checked separately in xsd_test.go).
func TestMatrix_ExportsCleanly(t *testing.T) {
	for _, c := range matrixDocs() {
		t.Run(c.name, func(t *testing.T) {
			_ = mustExport(t, c.sales, c.stock, c.work, c.payments)
		})
	}
}
