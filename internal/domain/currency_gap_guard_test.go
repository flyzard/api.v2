package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestCurrencyAmountMismatch_NoSeriesGap proves that a currency-amount mismatch
// is rejected BEFORE issueCommon advances the series counter. Previously the
// guard ran after issueCommon, which consumed a sequence number on every mismatch
// — leaving a gap in the hash chain. The fix moves the check into the existing
// Currency != nil block, before issueCommon is called.
func TestCurrencyAmountMismatch_NoSeriesGap(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)

	series := mustVal(NewSeries("FT2026GAP", FT))
	if err := series.RegisterWithAT("BCDFGH37", seriesT0); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}

	if series.LastNum != 0 {
		t.Fatalf("precondition: series.LastNum = %d, want 0", series.LastNum)
	}
	if series.LastHash != "" {
		t.Fatalf("precondition: series.LastHash = %q, want empty", series.LastHash)
	}

	draft := &DraftSalesInvoice{}
	draft.DocumentType = FT
	draft.Customer = Customer{
		CustomerID:    uuid.New(),
		AccountID:     "ACC-GAP",
		CustomerTaxID: "500000000",
		CompanyName:   "Cliente Gap Lda.",
		BillingAddress: Address{
			AddressDetail: "Rua de Teste 1",
			City:          "Lisboa",
			PostalCode:    "1000-001",
			Country:       "PT",
		},
	}
	draft.Series = series
	draft.Date = now
	draft.Lines = []DocumentLine{normalVATLine(now)}
	// normalVATLine: 10.00 net + 23% VAT → gross = 12.30
	// Set a currency amount that deliberately does NOT match (1.00 != 12.30).
	// Date matches the invoice date exactly to avoid tripping the rate-date guard.
	draft.Currency = &Currency{
		Code:         "USD",
		Amount:       mustVal(NewMoney(1.00)), // wrong: gross is 12.30
		ExchangeRate: mustVal(NewExchangeRate(1.085)),
		Date:         now, // same instant as draft.Date → same Lisbon calendar day
	}

	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}
	_, err := IssueSalesInvoice(draft, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)

	if err == nil {
		t.Fatal("IssueSalesInvoice succeeded, want currency amount mismatch error")
	}
	if !strings.Contains(err.Error(), "currency amount") {
		t.Fatalf("error = %v, want message containing \"currency amount\"", err)
	}

	// The series counter must NOT have advanced — the fix ensures the guard runs
	// before issueCommon (which calls series.AppendIssue), so no sequence gap.
	if series.LastNum != 0 {
		t.Errorf("series.LastNum = %d after failed issue, want 0 (no gap)", series.LastNum)
	}
	if series.LastHash != "" {
		t.Errorf("series.LastHash = %q after failed issue, want empty (no gap)", series.LastHash)
	}
}
