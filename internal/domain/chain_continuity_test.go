package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// capturingSigner records each canonical string it is asked to sign, then
// delegates to the deterministic stub so the produced hash passes Hash.Validate.
type capturingSigner struct{ inputs []string }

func (c *capturingSigner) Sign(canonical string) (string, string, error) {
	c.inputs = append(c.inputs, canonical)
	return m16StubSigner{}.Sign(canonical)
}

// TestHashChain_GenesisAndContinuity pins the per-series hash chain itself
// (Portaria 363/2010): the first document signs over an EMPTY previous hash
// (trailing ';'), the second signs over the first's full Hash, and the series
// head advances to the latest hash. Today only covered indirectly.
func TestHashChain_GenesisAndContinuity(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	regTime := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	cust := Customer{
		CustomerID:    uuid.New(),
		AccountID:     "ACC-PT",
		CustomerTaxID: "500000000",
		CompanyName:   "Cliente PT Lda.",
		BillingAddress: Address{
			AddressDetail: "Rua de Teste 1", City: "Lisboa", PostalCode: "1000-001", Country: "PT",
		},
	}
	series := mustVal(NewSeries("A2026", FT))
	if err := series.RegisterWithAT("AAAABBBB", regTime); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}
	sign := &capturingSigner{}

	newDraft := func(date time.Time) *DraftSalesInvoice {
		d := &DraftSalesInvoice{}
		d.DocumentType = FT
		d.Customer = cust
		d.Series = series
		d.Date = date
		d.Lines = []DocumentLine{normalVATLine(date)}
		return d
	}

	day := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	inv1, err := IssueSalesInvoice(newDraft(day), &series, sign, "tester", now, IssueOptions{}, qr)
	if err != nil {
		t.Fatalf("issue 1: %v", err)
	}
	inv2, err := IssueSalesInvoice(newDraft(day), &series, sign, "tester", now, IssueOptions{}, qr)
	if err != nil {
		t.Fatalf("issue 2: %v", err)
	}

	if len(sign.inputs) != 2 {
		t.Fatalf("expected 2 canonical inputs, got %d", len(sign.inputs))
	}
	if !strings.HasSuffix(sign.inputs[0], ";") {
		t.Errorf("genesis canonical must end with an empty prevHash:\n%q", sign.inputs[0])
	}
	if !strings.Contains(sign.inputs[1], string(inv1.Hash)) {
		t.Errorf("doc 2 canonical must contain doc 1 Hash %q:\n%q", inv1.Hash, sign.inputs[1])
	}
	if series.LastHash != string(inv2.Hash) {
		t.Errorf("series.LastHash = %q, want doc 2 Hash %q", series.LastHash, inv2.Hash)
	}
}
