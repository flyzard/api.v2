package app_test

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

const testTenantID = "tenant-1"

func mustVal[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// stubSigner mirrors the demo's deterministic non-RSA signer — enough to drive
// the hash-chain plumbing in tests.
type stubSigner struct{}

func (stubSigner) Sign(canonical string) (string, string, error) {
	a := sha512.Sum512([]byte(canonical))
	b := sha512.Sum512(a[:])
	return base64.StdEncoding.EncodeToString(append(a[:], b[:]...)), "1", nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// mapTenantStore resolves tenants by ID from a static map.
type mapTenantStore struct{ tenants map[string]app.Tenant }

func (s mapTenantStore) Resolve(_ context.Context, id string) (app.Tenant, error) {
	t, ok := s.tenants[id]
	if !ok {
		return app.Tenant{}, app.ErrNotFound
	}
	return t, nil
}

func oneTenant(t app.Tenant) mapTenantStore {
	return mapTenantStore{tenants: map[string]app.Tenant{t.ID: t}}
}

func testLisbon() *time.Location { return mustVal(time.LoadLocation("Europe/Lisbon")) }

func testNow() time.Time { return time.Date(2026, 5, 22, 9, 0, 0, 0, testLisbon()) }

func testTenant() app.Tenant { return testTenantNamed(testTenantID) }

func testTenantNamed(id string) app.Tenant {
	addr := mustVal(domain.NewAddress("Rua dos Programadores 1", "Lisboa", "1000-100", "PT"))
	company := mustVal(domain.NewCompany(domain.Company{
		NIF:        "500000000",
		Name:       "Demo Faturação Lda.",
		Address:    addr,
		FiscalYear: 2026,
		StartMonth: 1,
		EACCode:    "47190",
		Active:     true,
	}))
	return app.Tenant{
		ID:       id,
		Company:  company,
		CommMode: app.CommRealtime,
	}
}

func testSoftware() config.SoftwareIdentity {
	return config.SoftwareIdentity{
		ProducerTaxID:     "500000000",
		SoftwareName:      "DemoInvoicer",
		ProducerName:      "Demo Lda.",
		Version:           "1.0",
		CertificateNumber: "9999",
	}
}

func activeFTSeries(now time.Time) domain.Series {
	s := mustVal(domain.NewSeries("FT2026", domain.FT))
	if err := s.RegisterWithAT("ATCODE01", now); err != nil {
		panic(err)
	}
	return s
}

// ftDraft builds a valid one-line FT draft. Series must be set on the draft
// (domain validation requires a non-empty series ID).
func ftDraft(series domain.Series, date time.Time) domain.DraftSalesInvoice {
	cust := *mustVal(domain.NewCustomer(
		"ACC-PT-001", "503504564", "Acme Lda.",
		mustVal(domain.NewAddress("Rua das Flores 12", "Lisboa", "1000-001", "PT")),
		false,
	))
	prod := mustVal(domain.NewProduct(domain.Product{
		ProductCode:        "P-NOR",
		ProductType:        domain.ProductTypeGoods,
		ProductDescription: "Auriculares Bluetooth",
		ProductNumberCode:  "P-NOR",
		Unit:               domain.UnitPiece,
		Active:             true,
	}))
	user := mustVal(domain.NewUser("issuer@demo.pt", "Maria Operadora"))
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: domain.FT,
			Customer:     cust,
			Date:         date,
			IssuedBy:     user,
		},
		Series: series,
	}
	cd.AddLine(domain.DocumentLine{
		Product:      prod,
		Quantity:     mustVal(domain.NewQuantity(2)),
		UnitPrice:    mustVal(domain.NewMoney(30.00)),
		TaxPointDate: date,
		Tax:          mustVal(domain.NewVATLineTax(domain.PT, domain.TaxNormal, "", "")),
	})
	return domain.DraftSalesInvoice{
		CommonDraftDocument: cd,
		SalesInvoiceFields:  domain.SalesInvoiceFields{},
	}
}

// newFixture wires the full fake stack with one active FT series seeded for the
// default tenant.
func newFixture() (*app.Services, *memstore.Store) {
	now := testNow()
	store := memstore.New()
	store.SeedSeries(testTenantID, activeFTSeries(now))
	svc := app.New(app.Deps{
		Tenants:  oneTenant(testTenant()),
		UoW:      store,
		Clock:    fixedClock{t: now},
		Signer:   stubSigner{},
		Software: testSoftware(),
	})
	return svc, store
}

// TestFixtureDraftIssues proves the fixture draft is genuinely issuable through
// the domain, independent of the service — so service-test failures cannot be
// blamed on a malformed fixture.
func TestFixtureDraftIssues(t *testing.T) {
	now := testNow()
	series := activeFTSeries(now)
	draft := ftDraft(series, now)
	doc, err := domain.IssueSalesInvoice(
		&draft, &series, stubSigner{}, "src", now,
		domain.IssueOptions{},
		domain.QRConfig{IssuerNIF: testTenant().Company.NIF, CertificateNumber: "9999"},
	)
	if err != nil {
		t.Fatalf("fixture draft must issue cleanly: %v", err)
	}
	if doc.Number.Seq != 1 {
		t.Fatalf("seq = %d, want 1", doc.Number.Seq)
	}
}

// ── multi-family fixtures (work / stock / payment / ND) ──────────────────────

// activeSeries seeds-ready an AT-registered series of any type.
func activeSeries(id string, dt domain.DocumentType, now time.Time) domain.Series {
	s := mustVal(domain.NewSeries(id, dt))
	if err := s.RegisterWithAT("ATCODE01", now); err != nil {
		panic(err)
	}
	return s
}

// newFixtureSeries wires the fake stack with an explicit set of seeded series.
func newFixtureSeries(now time.Time, series ...domain.Series) (*app.Services, *memstore.Store) {
	store := memstore.New()
	for _, s := range series {
		store.SeedSeries(testTenantID, s)
	}
	svc := app.New(app.Deps{
		Tenants:  oneTenant(testTenant()),
		UoW:      store,
		Clock:    fixedClock{t: now},
		Signer:   stubSigner{},
		Software: testSoftware(),
	})
	return svc, store
}

func testCustomer() domain.Customer {
	return *mustVal(domain.NewCustomer(
		"ACC-PT-001", "503504564", "Acme Lda.",
		mustVal(domain.NewAddress("Rua das Flores 12", "Lisboa", "1000-001", "PT")),
		false,
	))
}

func testProduct() domain.Product {
	return mustVal(domain.NewProduct(domain.Product{
		ProductCode:        "P-NOR",
		ProductType:        domain.ProductTypeGoods,
		ProductDescription: "Auriculares Bluetooth",
		ProductNumberCode:  "P-NOR",
		Unit:               domain.UnitPiece,
		Active:             true,
	}))
}

func testUser() domain.User { return mustVal(domain.NewUser("issuer@demo.pt", "Maria Operadora")) }

func normalTax() domain.LineTax {
	return mustVal(domain.NewVATLineTax(domain.PT, domain.TaxNormal, "", ""))
}

func testLine(date time.Time) domain.DocumentLine {
	return domain.DocumentLine{
		Product:      testProduct(),
		Quantity:     mustVal(domain.NewQuantity(2)),
		UnitPrice:    mustVal(domain.NewMoney(30.00)),
		TaxPointDate: date,
		Tax:          normalTax(),
	}
}

func commonDraft(dt domain.DocumentType, series domain.Series, date time.Time, line domain.DocumentLine) domain.CommonDraftDocument {
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: dt,
			Customer:     testCustomer(),
			Date:         date,
			IssuedBy:     testUser(),
		},
		Series: series,
	}
	cd.AddLine(line)
	return cd
}

func workDraft(series domain.Series, date time.Time) domain.DraftWorkDocument {
	return domain.DraftWorkDocument{CommonDraftDocument: commonDraft(domain.NE, series, date, testLine(date))}
}

func stockDraft(series domain.Series, date time.Time) domain.DraftStockMovement {
	from := mustVal(domain.NewAddress("Polo Logístico Sul", "Setúbal", "2900-100", "PT"))
	to := mustVal(domain.NewAddress("Rua das Flores 12", "Lisboa", "1000-001", "PT"))
	return domain.DraftStockMovement{
		CommonDraftDocument: commonDraft(domain.GR, series, date, testLine(date)),
		StockMovementFields: domain.StockMovementFields{
			MovementStartTime: date.Add(2 * time.Hour),
			ShipFrom:          &domain.ShippingPoint{Address: &from},
			ShipTo:            &domain.ShippingPoint{Address: &to},
		},
	}
}

// paymentDraftRG builds an advance receipt (no specific source invoice) plus its
// caller-supplied totals.
func paymentDraftRG(date time.Time) (domain.PaymentDraft, domain.PaymentTotals) {
	advance := mustVal(domain.NewMoney(50.00))
	draft := domain.PaymentDraft{
		Type:            domain.RG,
		TransactionDate: date,
		Customer:        testCustomer(),
		SourceID:        "issuer@demo.pt",
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismCash,
			Amount:    advance,
			Date:      date,
		}},
		Lines: []domain.PaymentLine{{
			LineNumber: 1,
			SourceDocuments: []domain.SourceDocumentID{{
				OriginatingON: "Adiantamento",
				InvoiceDate:   date,
				Description:   "Adiantamento por conta de serviços futuros",
			}},
			Movement: domain.CreditAmount{Value: advance},
		}},
	}
	totals := domain.PaymentTotals{NetTotal: advance, TaxPayable: 0, GrossTotal: advance}
	return draft, totals
}

// ndDraft builds a debit note adjusting the price of ref's single P-NOR line
// (quantity must match the originating line for the ND product-set invariant).
func ndDraft(series domain.Series, date time.Time, ref domain.SalesInvoice) domain.DraftSalesInvoice {
	line := testLine(date)
	line.Quantity = ref.Lines[0].Quantity
	line.UnitPrice = mustVal(domain.NewMoney(2.00))
	line.References = []domain.DocReference{{Reference: ref.Number.Format(), Reason: "Acerto de preço"}}
	return domain.DraftSalesInvoice{CommonDraftDocument: commonDraft(domain.ND, series, date, line)}
}
