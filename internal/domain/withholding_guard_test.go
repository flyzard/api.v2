package domain

import (
	"strings"
	"testing"
	"time"
)

func TestIssueSalesInvoice_RejectsWithholdingOverGross(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	d := gdDraft(t, nil)
	series := mustVal(NewSeries("WH2026", FT))
	if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
		t.Fatal(err)
	}
	d.Series = series
	d.CalculateTotals()
	d.WithholdingTax = []WithholdingTax{{Amount: d.Totals.GrossTotal + Money(1*scale)}}

	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}
	_, err := IssueSalesInvoice(d, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
	if err == nil || !strings.Contains(err.Error(), "withholding") {
		t.Fatalf("want withholding-over-gross rejection, got %v", err)
	}
	if series.LastNum != 0 {
		t.Fatalf("series advanced on rejected issue: LastNum=%d", series.LastNum)
	}
}
