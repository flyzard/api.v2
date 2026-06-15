package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// validPaymentDraft builds a minimal RG draft that passes Validate.
func validPaymentDraft() *PaymentDraft {
	date := time.Date(2026, 6, 5, 10, 0, 0, 0, lisbonLocation)
	return &PaymentDraft{
		Type:            RG,
		TransactionDate: date,
		Customer: Customer{
			CustomerID:    uuid.New(),
			AccountID:     "ACC-RG",
			CustomerTaxID: "555555550",
			CompanyName:   "Cliente RG Lda.",
			BillingAddress: Address{
				AddressDetail: "Rua de Teste 2",
				City:          "Porto",
				PostalCode:    "4000-001",
				Country:       "PT",
			},
		},
		SourceID: "tester",
		Lines: []PaymentLine{{
			LineNumber: 1,
			SourceDocuments: []SourceDocumentID{{
				OriginatingON: "FT FT2026/1",
				InvoiceDate:   date.AddDate(0, 0, -1),
			}},
			Movement: CreditAmount{Value: Money(10 * scale)},
		}},
	}
}

// TestPaymentMethodRejectsZeroAmount aligns PaymentMethod with FRPayment
// (sales_invoice.go): a settlement row that moves no money is meaningless —
// both families require a positive amount. Mechanism rules stay divergent on
// purpose: the XSD makes PaymentMechanism optional on receipts, while FR rows
// must state how the invoice was paid.
func TestPaymentMethodRejectsZeroAmount(t *testing.T) {
	m := PaymentMethod{
		Mechanism: PaymentMechanismCash,
		Amount:    0,
		Date:      time.Date(2026, 6, 5, 0, 0, 0, 0, lisbonLocation),
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("error = %v, want positive-amount rejection", err)
	}
	m.Amount = Money(10 * scale)
	if err := m.Validate(); err != nil {
		t.Fatalf("valid method rejected: %v", err)
	}
}

// TestPaymentDraftLineNumbers pins the same gap/collision rules the sales
// family enforces (document.go AddLine + Validate): SAF-T LineNumber is a
// positive integer, and duplicates within a document are invalid. The payment
// path has no AddLine, so Validate is the only gate before the projector
// copies LineNumber verbatim into the XML.
func TestPaymentDraftLineNumbers(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := validPaymentDraft().Validate(); err != nil {
			t.Fatalf("valid draft rejected: %v", err)
		}
	})

	t.Run("zero-line-number", func(t *testing.T) {
		d := validPaymentDraft()
		d.Lines[0].LineNumber = 0
		err := d.Validate()
		if err == nil || !strings.Contains(err.Error(), "line number") {
			t.Fatalf("error = %v, want line number >= 1 rejection", err)
		}
	})

	t.Run("duplicate-line-number", func(t *testing.T) {
		d := validPaymentDraft()
		dup := d.Lines[0]
		d.Lines = append(d.Lines, dup)
		err := d.Validate()
		if err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("error = %v, want duplicate LineNumber rejection", err)
		}
	})
}
