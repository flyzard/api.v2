package pdf

import (
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"github.com/google/uuid"
)

func mustMoney(t *testing.T, euros float64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(euros)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustQty(t *testing.T, v float64) domain.Quantity {
	t.Helper()
	q, err := domain.NewQuantity(v)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func mustPercent(t *testing.T, v float64) domain.Percent {
	t.Helper()
	p, err := domain.NewPercent(v)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustRate(t *testing.T, rate float64) domain.ExchangeRate {
	t.Helper()
	r, err := domain.NewExchangeRate(rate)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func fixtureCustomer() domain.Customer {
	return domain.Customer{
		CustomerID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		CompanyName:   "Cliente Exemplo Lda",
		CustomerTaxID: "123456789",
		BillingAddress: domain.Address{
			AddressDetail: "Avenida Teste 42",
			City:          "Porto",
			PostalCode:    "4000-001",
			Country:       "PT",
		},
	}
}

func fixtureDate() time.Time {
	return time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
}

func fixtureLine(t *testing.T) domain.DocumentLine {
	return domain.DocumentLine{
		LineNumber: 1,
		Product: domain.Product{
			ProductCode:        "P001",
			ProductDescription: "Serviço de consultoria",
		},
		Quantity:     mustQty(t, 2),
		UnitPrice:    mustMoney(t, 100),
		TaxPointDate: fixtureDate(),
		Tax: domain.VATTax{Rate: domain.TaxRate{
			Region:   domain.PT,
			Category: domain.TaxNormal,
			Value:    mustPercent(t, 23),
		}},
	}
}

func fixtureNC(t *testing.T) domain.SalesInvoice {
	inv := fixtureFT(t)
	inv.Number = domain.DocNumber{Type: domain.NC, Series: "A2026", Seq: 7}
	inv.DocumentType = domain.NC
	line := fixtureLine(t)
	line.References = []domain.DocReference{{Reference: "FT A2026/42", Reason: "Devolução de mercadoria"}}
	inv.Lines = []domain.DocumentLine{line}
	return inv
}

func fixtureFTCancelled(t *testing.T) domain.SalesInvoice {
	inv := fixtureFT(t)
	inv.Status = domain.StatusCancelled
	inv.Reason = "Erro de faturação"
	return inv
}

func fixtureFTWithholding(t *testing.T) domain.SalesInvoice {
	inv := fixtureFT(t)
	inv.WithholdingTax = []domain.WithholdingTax{{
		Type: domain.WithholdingIRS, Description: "IRS 25%", Amount: mustMoney(t, 50),
	}}
	inv.Totals.AmountPayable = mustMoney(t, 196)
	return inv
}

func fixtureFR(t *testing.T) domain.SalesInvoice {
	inv := fixtureFT(t)
	inv.Number = domain.DocNumber{Type: domain.FR, Series: "A2026", Seq: 3}
	inv.DocumentType = domain.FR
	inv.Payments = []domain.FRPayment{{
		Mechanism: domain.PaymentMechanismCash, Amount: mustMoney(t, 246), Date: fixtureDate(),
	}}
	return inv
}

func fixtureFSAnonymous(t *testing.T) domain.SalesInvoice {
	inv := fixtureFT(t)
	inv.Number = domain.DocNumber{Type: domain.FS, Series: "A2026", Seq: 9}
	inv.DocumentType = domain.FS
	inv.Customer = domain.NewAnonymousCustomer()
	return inv
}

func fixtureFTExempt(t *testing.T) domain.SalesInvoice {
	t.Helper()
	inv := fixtureFT(t)
	line := fixtureLine(t)
	tx, err := domain.NewVATLineTax(domain.PT, domain.TaxExempt, domain.M07, "Isento artigo 9.º do CIVA")
	if err != nil {
		t.Fatal(err)
	}
	line.Tax = tx
	inv.Lines = []domain.DocumentLine{line}
	inv.Totals = domain.Totals{
		NetTotal: mustMoney(t, 200), TaxTotal: 0,
		GrossTotal: mustMoney(t, 200), AmountPayable: mustMoney(t, 200),
		Breakdown: domain.TaxBreakdown{{
			Region: domain.PT, Category: domain.TaxExempt, ExemptionCode: domain.M07,
			ExemptionDescription: domain.M07.Description(),
			Base:                 mustMoney(t, 200), Tax: 0,
		}},
	}
	return inv
}

func fixtureFT(t *testing.T) domain.SalesInvoice {
	t.Helper()
	return domain.SalesInvoice{
		IssuedDocument: domain.IssuedDocument{
			Number: domain.DocNumber{Type: domain.FT, Series: "A2026", Seq: 42},
			ATCUD:  domain.ATCUD("CSDF7T5H-42"),
			Hash:   domain.Hash("Abcdefghij1bcdefghij2bcdefghij3bcdefghijRESTOFHASHPADDINGTOLOOKREAL=="),
			Status: domain.StatusNormal,
			DocumentCore: domain.DocumentCore{
				DocumentType: domain.FT,
				Customer:     fixtureCustomer(),
				Date:         fixtureDate(),
				Lines:        []domain.DocumentLine{fixtureLine(t)},
				Totals: domain.Totals{
					NetTotal:      mustMoney(t, 200),
					TaxTotal:      mustMoney(t, 46),
					GrossTotal:    mustMoney(t, 246),
					AmountPayable: mustMoney(t, 246),
					Breakdown: domain.TaxBreakdown{{
						Region:   domain.PT,
						Category: domain.TaxNormal,
						Base:     mustMoney(t, 200),
						Tax:      mustMoney(t, 46),
					}},
				},
			},
			QRPayload: "A:555555550*B:123456789*C:PT*D:FT*E:N*F:20260510*G:FT A2026/42*H:CSDF7T5H-42*I1:PT*I7:200.00*I8:46.00*N:46.00*O:246.00*Q:Abcd*R:9999",
		},
	}
}
