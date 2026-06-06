package pdf

import (
	"testing"

	"github.com/johnfercher/maroto/v2/pkg/test"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func fixtureRG(t *testing.T) domain.Payment {
	t.Helper()
	settle := mustMoney(t, 246)
	return domain.Payment{
		Number:          domain.DocNumber{Type: domain.RG, Series: "A2026", Seq: 2},
		ATCUD:           domain.ATCUD("CSDF7T5H-2"),
		Type:            domain.RG,
		Status:          domain.StatusNormal,
		TransactionDate: fixtureDate(),
		SystemEntryDate: fixtureDate(),
		Customer:        fixtureCustomer(),
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismBankTransfer, Amount: mustMoney(t, 246), Date: fixtureDate(),
		}},
		Lines: []domain.PaymentLine{{
			LineNumber: 1,
			SourceDocuments: []domain.SourceDocumentID{{
				OriginatingON: "FT A2026/42", InvoiceDate: fixtureDate(),
			}},
			SettlementAmount: &settle,
			Movement:         domain.CreditAmount{Value: mustMoney(t, 246)},
		}},
		PaymentTotals: domain.PaymentTotals{
			NetTotal: mustMoney(t, 200), TaxPayable: mustMoney(t, 46), GrossTotal: mustMoney(t, 246),
		},
	}
}

func TestBuildPayment_Structure(t *testing.T) {
	eng, err := buildPayment(fixtureRG(t), validMeta())
	if err != nil {
		t.Fatal(err)
	}
	test.New(t).Assert(eng.GetStructure()).Equals("rg_basic.json")
}

func fixtureRGWithholding(t *testing.T) domain.Payment {
	t.Helper()
	p := fixtureRG(t)
	p.WithholdingTax = []domain.WithholdingTax{{
		Type: domain.WithholdingIRS, Description: "IRS 25%", Amount: mustMoney(t, 50),
	}}
	return p
}

func TestBuildPayment_Withholding_Structure(t *testing.T) {
	eng, err := buildPayment(fixtureRGWithholding(t), validMeta())
	if err != nil {
		t.Fatal(err)
	}
	test.New(t).Assert(eng.GetStructure()).Equals("rg_withholding.json")
}

func TestRenderPayment_ProducesPDF(t *testing.T) {
	b, err := RenderPayment(fixtureRG(t), validMeta())
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 4 || string(b[:4]) != "%PDF" {
		t.Fatalf("not a PDF")
	}
}
