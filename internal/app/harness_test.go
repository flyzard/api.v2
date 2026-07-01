package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/adapter/signing"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

var testLisbonLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("cannot load Europe/Lisbon: " + err.Error())
	}
	return loc
}()

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { c.t = c.t.Add(time.Minute); return c.t }

type oneTenant struct{ tn app.Tenant }

func (o oneTenant) Resolve(_ context.Context, id string) (app.Tenant, error) {
	if id != o.tn.ID {
		return app.Tenant{}, app.ErrNotFound
	}
	return o.tn, nil
}

func testCompany(t *testing.T) domain.Company {
	t.Helper()
	addr, err := domain.NewAddress("Travessa Serradinha, 46", "BENEDITA", "2475-116", "PT")
	if err != nil {
		t.Fatalf("address: %v", err)
	}
	co, err := domain.NewCompany(domain.Company{
		NIF: "519348761", Name: "AVENIDA DO CODIGO LDA", TradeName: "Faturly",
		Address: addr, FiscalYear: 2026, StartMonth: 1, EACCode: "47190", Active: true,
	})
	if err != nil {
		t.Fatalf("company: %v", err)
	}
	return co
}

func newTestServices(t *testing.T) (*app.Services, string) {
	t.Helper()
	pem, err := os.ReadFile("../adapter/signing/testdata/sign_key.pem")
	if err != nil {
		t.Fatalf("read test key: %v", err)
	}
	signer, err := signing.NewRSASigner(pem, 1)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	store := memstore.New()
	tenant := app.Tenant{ID: "t1", Company: testCompany(t)} // CommMode default; AllowUnknownAllocSource stays false
	for _, dt := range []domain.DocumentType{domain.FT, domain.FS, domain.FR, domain.NC, domain.ND, domain.OR, domain.PF, domain.GT, domain.RC, domain.RG} {
		s, _ := domain.NewSeries(string(dt)+"2026", dt)
		_ = s.RegisterWithAT("ATCODE01", time.Date(2026, 4, 1, 9, 0, 0, 0, testLisbonLoc))
		store.SeedSeries(tenant.ID, s)
	}
	svc := app.New(app.Deps{
		Tenants:  oneTenant{tenant},
		UoW:      store,
		Clock:    &fixedClock{t: time.Date(2026, 5, 1, 9, 0, 0, 0, testLisbonLoc)},
		Signer:   signer,
		Software: config.SoftwareIdentity{CertificateNumber: "9999"},
	})
	return svc, tenant.ID
}

// sampleFTInput returns a valid IssueInvoiceInput for a FT on series "FT2026"
// with a real-NIF customer and one goods line.
func sampleFTInput() app.IssueInvoiceInput {
	return app.IssueInvoiceInput{
		DocType:  app.DocFT,
		SeriesID: "FT2026",
		SourceID: "SRC-001",
		IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
		Date:     "2026-05-01",
		Customer: app.CustomerInput{
			TaxID:   "502819472",
			Name:    "Restaurante O Cantinho, Lda",
			Country: "PT",
			Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
		},
		Lines: []app.LineInput{{
			ProductCode:        "P003",
			ProductType:        app.ProductGoods,
			ProductDescription: "Produto de teste",
			ProductNumberCode:  "P003",
			Unit:               app.UnitPiece,
			Quantity:           1,
			UnitPriceCents:     10000, // 100.00 EUR
			TaxPointDate:       "2026-05-01",
			Tax: &app.LineTaxInput{
				Kind:     "VAT",
				Region:   app.RegionPT,
				Category: app.RateNormal,
			},
		}},
		Idem: app.IdempotencyKey{Key: "ft-sample-001", Fingerprint: "fp-ft-001"},
	}
}

// sampleNCInput returns a valid IssueInvoiceInput for a NC on series "NC2026"
// whose single line references the given FT number. Same customer as sampleFTInput.
func sampleNCInput(ref string) app.IssueInvoiceInput {
	return app.IssueInvoiceInput{
		DocType:  app.DocNC,
		SeriesID: "NC2026",
		SourceID: "SRC-002",
		IssuedBy: app.UserInput{Email: "op@faturly.pt", Name: "Operador"},
		Date:     "2026-05-01",
		Customer: app.CustomerInput{
			TaxID:   "502819472",
			Name:    "Restaurante O Cantinho, Lda",
			Country: "PT",
			Address: &app.AddressInput{Detail: "Av. da Boavista, 1200", City: "Porto", PostalCode: "4100-130"},
		},
		Lines: []app.LineInput{{
			ProductCode:        "P003",
			ProductType:        app.ProductGoods,
			ProductDescription: "Produto de teste",
			ProductNumberCode:  "P003",
			Unit:               app.UnitPiece,
			Quantity:           1,
			UnitPriceCents:     10000, // 100.00 EUR
			TaxPointDate:       "2026-05-01",
			Tax: &app.LineTaxInput{
				Kind:     "VAT",
				Region:   app.RegionPT,
				Category: app.RateNormal,
			},
			References: []app.DocRefInput{{Reference: ref, Reason: "devolução"}},
		}},
		Idem: app.IdempotencyKey{Key: "nc-sample-001", Fingerprint: "fp-nc-001"},
	}
}
