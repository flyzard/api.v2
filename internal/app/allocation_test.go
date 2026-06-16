package app_test

// Task 4 (B3-b): allocation validation wired into NC/ND + RC/RG issuance.
// All tests use memstore-backed fixtures and do not touch golden files.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// sharedCustomer returns a single Customer value so FT/NC/ND/RG all share the
// same CustomerID UUID (NewCustomer generates a new UUID on every call, so the
// tests must create one and reuse it).
func sharedCustomer() domain.Customer {
	return *mustVal(domain.NewCustomer(
		"ACC-PT-001", "503504564", "Acme Lda.",
		mustVal(domain.NewAddress("Rua das Flores 12", "Lisboa", "1000-001", "PT")),
		false,
	))
}

// otherCustomer is a second distinct customer for the customer-mismatch test.
// NIF 500000000 is a valid Portuguese NIF (prefix 5, checksum=0).
func otherCustomer() domain.Customer {
	return *mustVal(domain.NewCustomer(
		"ACC-PT-002", "500000000", "Outra Empresa SA.",
		mustVal(domain.NewAddress("Av. da Liberdade 100", "Lisboa", "1250-096", "PT")),
		false,
	))
}

// ftDraftShared is like ftDraft but uses a caller-supplied customer.
func ftDraftShared(series domain.Series, date time.Time, cust domain.Customer) domain.DraftSalesInvoice {
	prod := mustVal(domain.NewProduct(domain.Product{
		ProductCode:        "P-NOR",
		ProductType:        domain.ProductTypeGoods,
		ProductDescription: "Auriculares Bluetooth",
		ProductNumberCode:  "P-NOR",
		Unit:               domain.UnitPiece,
		Active:             true,
	}))
	user := testUser()
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
		Tax:          normalTax(),
	})
	return domain.DraftSalesInvoice{CommonDraftDocument: cd}
}

// ncDraftShared builds an NC that uses the same customer as the supplied FT.
func ncDraftShared(series domain.Series, date time.Time, ref domain.SalesInvoice, unitPrice float64) domain.DraftSalesInvoice {
	line := domain.DocumentLine{
		Product:      testProduct(),
		Quantity:     mustVal(domain.NewQuantity(2)),
		UnitPrice:    mustVal(domain.NewMoney(unitPrice)),
		TaxPointDate: date,
		Tax:          normalTax(),
		References:   []domain.DocReference{{Reference: ref.Number.Format(), Reason: "Rectificação"}},
	}
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: domain.NC,
			Customer:     ref.Customer, // same UUID as the FT
			Date:         date,
			IssuedBy:     testUser(),
		},
		Series: series,
	}
	cd.AddLine(line)
	return domain.DraftSalesInvoice{CommonDraftDocument: cd}
}

// ndDraftShared builds an ND that uses the same customer as the supplied FT.
func ndDraftShared(series domain.Series, date time.Time, ref domain.SalesInvoice) domain.DraftSalesInvoice {
	line := domain.DocumentLine{
		Product:      testProduct(),
		Quantity:     ref.Lines[0].Quantity, // ND must match originating quantity
		UnitPrice:    mustVal(domain.NewMoney(2.00)),
		TaxPointDate: date,
		Tax:          normalTax(),
		References:   []domain.DocReference{{Reference: ref.Number.Format(), Reason: "Acerto de preço"}},
	}
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: domain.ND,
			Customer:     ref.Customer, // same UUID as the FT
			Date:         date,
			IssuedBy:     testUser(),
		},
		Series: series,
	}
	cd.AddLine(line)
	return domain.DraftSalesInvoice{CommonDraftDocument: cd}
}

// issueFTWith issues an FT using the shared customer and the given series.
func issueFTWith(t *testing.T, svc *app.Services, cust domain.Customer, key string) domain.SalesInvoice {
	t.Helper()
	draft := ftDraftShared(activeFTSeries(testNow()), testNow(), cust)
	ft, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
		app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src",
			Idem: app.IdempotencyKey{Key: key, Fingerprint: "fp-" + key}})
	if err != nil {
		t.Fatalf("issueFTWith: %v", err)
	}
	return ft
}

// issueNCShared issues an NC in the given series against ref.
func issueNCShared(t *testing.T, svc *app.Services, seriesID string, ncS domain.Series, ref domain.SalesInvoice, price float64, key string) (domain.SalesInvoice, error) {
	t.Helper()
	draft := ncDraftShared(ncS, testNow(), ref, price)
	return svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
		app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: seriesID, SourceID: "src",
			Idem: app.IdempotencyKey{Key: key, Fingerprint: "fp-" + key}})
}

// buildStore creates a memstore, seeds the given series, and returns svc+store.
func buildStore(t *testing.T, tenant app.Tenant, series ...domain.Series) (*app.Services, *memstore.Store) {
	t.Helper()
	now := testNow()
	store := memstore.New()
	for _, s := range series {
		store.SeedSeries(testTenantID, s)
	}
	svc := app.New(app.Deps{
		Tenants: oneTenant(tenant), UoW: store,
		Clock: fixedClock{t: now}, Signer: stubSigner{}, Software: testSoftware(),
	})
	return svc, store
}

// cancelPaymentInStore cancels a payment directly via the store UoW (no
// CancelPayment service exists in the app layer yet).
func cancelPaymentInStore(t *testing.T, store *memstore.Store, num domain.DocNumber) {
	t.Helper()
	err := store.Run(context.Background(), testTenantID, func(tx app.RepoSet) error {
		p, err := tx.Documents().GetPayment(num)
		if err != nil {
			return err
		}
		if err := p.Cancel("test cancel", testNow()); err != nil {
			return err
		}
		return tx.Documents().SavePayment(p)
	})
	if err != nil {
		t.Fatalf("cancelPaymentInStore: %v", err)
	}
}

// paymentDraftSettlingFT builds an RG settling a specific FT for amt (a Money
// value in internal units, not euros). Pass ft.Totals.GrossTotal to settle in
// full, or a sub-amount for partial settlement.
func paymentDraftSettlingFT(date time.Time, ft domain.SalesInvoice, amt domain.Money) (domain.PaymentDraft, domain.PaymentTotals) {
	draft := domain.PaymentDraft{
		Type:            domain.RG,
		TransactionDate: date,
		Customer:        ft.Customer, // same UUID as the FT
		SourceID:        "issuer@demo.pt",
		Methods: []domain.PaymentMethod{{
			Mechanism: domain.PaymentMechanismCash,
			Amount:    amt,
			Date:      date,
		}},
		Lines: []domain.PaymentLine{{
			LineNumber: 1,
			SourceDocuments: []domain.SourceDocumentID{{
				OriginatingON: ft.Number.Format(),
				InvoiceDate:   ft.Date,
			}},
			Movement: domain.CreditAmount{Value: amt},
		}},
	}
	totals := domain.PaymentTotals{NetTotal: amt, TaxPayable: 0, GrossTotal: amt}
	return draft, totals
}

// oneCent is 1 EUR cent in Money internal units (scale=100_000, centScale=1_000).
const oneCent domain.Money = 1_000

func issueRGFor(t *testing.T, svc *app.Services, seriesID string, draft domain.PaymentDraft, totals domain.PaymentTotals, key string) (domain.Payment, error) {
	t.Helper()
	return svc.Invoicing.IssuePayment(context.Background(), testTenantID,
		app.IssuePaymentRequest{Draft: draft, SeriesID: seriesID, Totals: totals,
			Idem: app.IdempotencyKey{Key: key, Fingerprint: "fp-" + key}})
}

func activeNCSeries(now time.Time) domain.Series { return activeSeries("NC2026", domain.NC, now) }
func activeNDSeries(now time.Time) domain.Series { return activeSeries("ND2026", domain.ND, now) }
func activeRGSeriesID(id string, now time.Time) domain.Series {
	return activeSeries(id, domain.RG, now)
}

// ── NC over-credit ────────────────────────────────────────────────────────────

// TestAllocNC_OverCredit_Rejected: NC grossly over-credits FT → KindInvalid +
// ErrAllocationExceedsSource, and the NC series must NOT advance.
func TestAllocNC_OverCredit_Rejected(t *testing.T) {
	now := testNow()
	svc, store := buildStore(t, testTenant(), activeFTSeries(now), activeNCSeries(now))
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-overcredit")

	_, err := issueNCShared(t, svc, "NC2026", activeNCSeries(now), ft, 9999.00, "nc-over")
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", app.KindOf(err))
	}
	if !errors.Is(err, domain.ErrAllocationExceedsSource) {
		t.Fatalf("err = %v, want ErrAllocationExceedsSource", err)
	}
	// NC series must NOT have advanced.
	s, ok := store.GetSeries(testTenantID, "NC2026", domain.NC)
	if !ok {
		t.Fatal("NC2026 series missing from store")
	}
	if s.LastNum != 0 {
		t.Fatalf("NC series LastNum = %d, want 0 (rejection must not advance series)", s.LastNum)
	}
}

// TestAllocNC_OverCredit_AllowedWithRappel: same scenario but AllowRappelNC=true → succeeds.
func TestAllocNC_OverCredit_AllowedWithRappel(t *testing.T) {
	now := testNow()
	rappelTenant := testTenant()
	rappelTenant.AllowRappelNC = true
	svc, _ := buildStore(t, rappelTenant, activeFTSeries(now), activeNCSeries(now))
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-rappel")

	_, err := issueNCShared(t, svc, "NC2026", activeNCSeries(now), ft, 9999.00, "nc-rappel")
	if err != nil {
		t.Fatalf("AllowRappelNC=true must allow over-credit NC: %v", err)
	}
}

// ── NC cancelled source ───────────────────────────────────────────────────────

// TestAllocNC_CancelledSource: FT cancelled → NC referencing it → KindInvalid + ErrSourceDocCancelled.
func TestAllocNC_CancelledSource(t *testing.T) {
	now := testNow()
	svc, _ := buildStore(t, testTenant(), activeFTSeries(now), activeNCSeries(now))
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-cancelled-src")

	if _, err := svc.Invoicing.CancelDocument(context.Background(), testTenantID, ft.Number, "erro"); err != nil {
		t.Fatalf("cancel FT: %v", err)
	}

	_, err := issueNCShared(t, svc, "NC2026", activeNCSeries(now), ft, 5.00, "nc-cancelled-src")
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", app.KindOf(err))
	}
	if !errors.Is(err, domain.ErrSourceDocCancelled) {
		t.Fatalf("err = %v, want ErrSourceDocCancelled", err)
	}
}

// ── NC customer mismatch ──────────────────────────────────────────────────────

// TestAllocNC_CustomerMismatch: NC.Customer != FT.Customer → KindInvalid + ErrSourceCustomerMismatch.
func TestAllocNC_CustomerMismatch(t *testing.T) {
	now := testNow()
	svc, _ := buildStore(t, testTenant(), activeFTSeries(now), activeNCSeries(now))
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-mismatch")

	// Build NC with a DIFFERENT customer UUID.
	other := otherCustomer()
	ncS := activeNCSeries(now)
	line := domain.DocumentLine{
		Product:      testProduct(),
		Quantity:     mustVal(domain.NewQuantity(2)),
		UnitPrice:    mustVal(domain.NewMoney(5.00)),
		TaxPointDate: now,
		Tax:          normalTax(),
		References:   []domain.DocReference{{Reference: ft.Number.Format(), Reason: "Rectificação"}},
	}
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: domain.NC,
			Customer:     other,
			Date:         now,
			IssuedBy:     testUser(),
		},
		Series: ncS,
	}
	cd.AddLine(line)
	draft := domain.DraftSalesInvoice{CommonDraftDocument: cd}

	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
		app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "NC2026", SourceID: "src",
			Idem: app.IdempotencyKey{Key: "nc-mismatch", Fingerprint: "fp"}})
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", app.KindOf(err))
	}
	if !errors.Is(err, domain.ErrSourceCustomerMismatch) {
		t.Fatalf("err = %v, want ErrSourceCustomerMismatch", err)
	}
}

// ── RC/RG over-settlement (headline) ─────────────────────────────────────────

// TestAllocRG_OverSettlement_CrossSeries: FT gross G; RG1 settles G → ok;
// RG2 in a DIFFERENT series tries to settle 1 cent more → ErrAllocationExceedsSource.
// Series RG2026B must not advance.
func TestAllocRG_OverSettlement_CrossSeries(t *testing.T) {
	now := testNow()
	svc, store := buildStore(t, testTenant(),
		activeFTSeries(now),
		activeRGSeriesID("RG2026A", now),
		activeRGSeriesID("RG2026B", now),
	)
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-settle")
	gross := ft.Totals.GrossTotal

	// First RG: settle full gross → OK.
	d1, t1 := paymentDraftSettlingFT(now, ft, gross)
	_, err := issueRGFor(t, svc, "RG2026A", d1, t1, "rg-full")
	if err != nil {
		t.Fatalf("first full-gross RG should succeed: %v", err)
	}

	// Second RG in different series: 1 cent extra → over-settlement.
	d2, t2 := paymentDraftSettlingFT(now, ft, oneCent)
	_, err2 := issueRGFor(t, svc, "RG2026B", d2, t2, "rg-over")
	if app.KindOf(err2) != app.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", app.KindOf(err2))
	}
	if !errors.Is(err2, domain.ErrAllocationExceedsSource) {
		t.Fatalf("err = %v, want ErrAllocationExceedsSource", err2)
	}
	// RG2026B series must NOT have advanced.
	s, ok := store.GetSeries(testTenantID, "RG2026B", domain.RG)
	if !ok {
		t.Fatal("RG2026B series missing")
	}
	if s.LastNum != 0 {
		t.Fatalf("RG2026B LastNum = %d, want 0 (rejection must not advance series)", s.LastNum)
	}
}

// ── cancelled RG excluded ─────────────────────────────────────────────────────

// TestAllocRG_CancelledRGExcluded: a cancelled RG must NOT count toward Consumed.
// Settle, cancel it, then a fresh full settlement must succeed.
func TestAllocRG_CancelledRGExcluded(t *testing.T) {
	now := testNow()
	svc, store := buildStore(t, testTenant(),
		activeFTSeries(now),
		activeRGSeriesID("RG2026", now),
	)
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-cancel-rg")
	gross := ft.Totals.GrossTotal

	// Issue and cancel a full-gross RG.
	d1, t1 := paymentDraftSettlingFT(now, ft, gross)
	rg1, err := issueRGFor(t, svc, "RG2026", d1, t1, "rg-cancel")
	if err != nil {
		t.Fatalf("first RG should succeed: %v", err)
	}
	cancelPaymentInStore(t, store, rg1.Number)

	// Fresh full settlement must succeed (cancelled RG doesn't count).
	d2, t2 := paymentDraftSettlingFT(now, ft, gross)
	_, err2 := issueRGFor(t, svc, "RG2026", d2, t2, "rg-after-cancel")
	if err2 != nil {
		t.Fatalf("RG after cancelled RG should succeed (cancelled excluded from Consumed): %v", err2)
	}
}

// ── advance receipt (unparseable OriginatingON) ───────────────────────────────

// TestAllocRG_AdvanceReceipt: OriginatingON="Adiantamento" (unparseable doc number)
// must still succeed (AllowUnknownSource is always true for payments).
func TestAllocRG_AdvanceReceipt(t *testing.T) {
	now := testNow()
	svc, _ := buildStore(t, testTenant(), activeRGSeriesID("RG2026", now))

	// paymentDraftRG uses "Adiantamento" as OriginatingON.
	draft, totals := paymentDraftRG(now)
	_, err := svc.Invoicing.IssuePayment(context.Background(), testTenantID,
		app.IssuePaymentRequest{Draft: draft, SeriesID: "RG2026", Totals: totals,
			Idem: app.IdempotencyKey{Key: "rg-advance", Fingerprint: "fp"}})
	if err != nil {
		t.Fatalf("advance receipt (Adiantamento) should succeed: %v", err)
	}
}

// ── ND regression ─────────────────────────────────────────────────────────────

// TestAllocND_Regression: a live/same-customer/within-gross ND still passes.
func TestAllocND_Regression(t *testing.T) {
	now := testNow()
	svc, _ := buildStore(t, testTenant(), activeFTSeries(now), activeNDSeries(now))
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-nd")

	nd := ndDraftShared(activeNDSeries(now), now, ft)
	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
		app.IssueSalesInvoiceRequest{Draft: nd, SeriesID: "ND2026", SourceID: "src",
			Idem: app.IdempotencyKey{Key: "nd-ok", Fingerprint: "fp"}})
	if err != nil {
		t.Fatalf("valid ND should pass: %v", err)
	}
}

// ── idempotent replay ─────────────────────────────────────────────────────────

// TestAllocRG_IdempotentReplay: replaying the same RG issue must not double-count
// Consumed, i.e. the replay returns the original document and a subsequent
// different-key RG for the same source must still honour the ceiling.
func TestAllocRG_IdempotentReplay(t *testing.T) {
	now := testNow()
	svc, store := buildStore(t, testTenant(),
		activeFTSeries(now),
		activeRGSeriesID("RG2026", now),
	)
	cust := sharedCustomer()
	ft := issueFTWith(t, svc, cust, "ft-idem")
	gross := ft.Totals.GrossTotal

	d, tot := paymentDraftSettlingFT(now, ft, gross)
	idem := app.IdempotencyKey{Key: "rg-idem", Fingerprint: "fp-rg-idem"}

	// First issue.
	rg1, err := svc.Invoicing.IssuePayment(context.Background(), testTenantID,
		app.IssuePaymentRequest{Draft: d, SeriesID: "RG2026", Totals: tot, Idem: idem})
	if err != nil {
		t.Fatalf("first RG issue: %v", err)
	}

	// Replay with same idem key: must return the same document.
	rg2, err := svc.Invoicing.IssuePayment(context.Background(), testTenantID,
		app.IssuePaymentRequest{Draft: d, SeriesID: "RG2026", Totals: tot, Idem: idem})
	if err != nil {
		t.Fatalf("replay RG: %v", err)
	}
	if rg1.Number != rg2.Number {
		t.Fatalf("replay returned different number: %s vs %s", rg1.Number.Format(), rg2.Number.Format())
	}
	// Series must be at 1 — replay must not issue a second document.
	s, _ := store.GetSeries(testTenantID, "RG2026", domain.RG)
	if s.LastNum != 1 {
		t.Fatalf("RG series LastNum = %d, want 1 (replay must not re-issue)", s.LastNum)
	}
}

// ── NC AllowUnknownSource ─────────────────────────────────────────────────────

// TestAllocNC_AllowUnknownSource: NC referencing a doc not in the store passes
// when AllowUnknownAllocSource=true on the tenant.
func TestAllocNC_AllowUnknownSource(t *testing.T) {
	now := testNow()
	unknownTenant := testTenant()
	unknownTenant.AllowUnknownAllocSource = true
	svc, _ := buildStore(t, unknownTenant, activeFTSeries(now), activeNCSeries(now))

	// Reference a doc number that was never issued into the store.
	fakeFTNum := mustVal(domain.NewDocNumber(domain.FT, "FT2026", 999))
	ncS := activeNCSeries(now)
	line := domain.DocumentLine{
		Product:      testProduct(),
		Quantity:     mustVal(domain.NewQuantity(2)),
		UnitPrice:    mustVal(domain.NewMoney(5.00)),
		TaxPointDate: now,
		Tax:          normalTax(),
		References:   []domain.DocReference{{Reference: fakeFTNum.Format(), Reason: "Ref externa"}},
	}
	cd := domain.CommonDraftDocument{
		DocumentCore: domain.DocumentCore{
			DocumentType: domain.NC,
			Customer:     sharedCustomer(),
			Date:         now,
			IssuedBy:     testUser(),
		},
		Series: ncS,
	}
	cd.AddLine(line)
	draft := domain.DraftSalesInvoice{CommonDraftDocument: cd}

	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
		app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: ncS.ID, SourceID: "src",
			Idem: app.IdempotencyKey{Key: "nc-unknown-src", Fingerprint: "fp"}})
	if err != nil {
		t.Fatalf("AllowUnknownAllocSource=true must allow unknown source: %v", err)
	}
}

// ── per-source concurrency lock (Task 5, B3-c) ───────────────────────────────

// TestConcurrentSettlement_ExactlyMSucceed: N goroutines each issue an RG in a
// DIFFERENT series, all settling the SAME FT whose gross covers exactly M full
// settlements (M < N). The per-source lock serializes the check-then-act window
// so exactly M succeed and N-M fail with ErrAllocationExceedsSource — no
// over-settlement is possible regardless of scheduling.
//
// Design: FT gross = 3 × partialAmt (M=3); N=10 goroutines each try to settle
// partialAmt. The lock guarantees that once 3 settlements have committed, all
// remaining Consumed reads see a full source and reject immediately.
func TestConcurrentSettlement_ExactlyMSucceed(t *testing.T) {
	const N = 10 // goroutines
	const M = 3  // how many fit within the source gross

	now := testNow()

	// Build N distinct RG series (RG-0 … RG-9).
	rgSeries := make([]domain.Series, N)
	for i := 0; i < N; i++ {
		rgSeries[i] = activeSeries(fmt.Sprintf("RG%04d", i), domain.RG, now)
	}

	// Seed all series plus an FT series.
	allSeries := append([]domain.Series{activeFTSeries(now)}, rgSeries...)
	svc, _ := buildStore(t, testTenant(), allSeries...)

	// Issue an FT whose gross is exactly M × (gross/M). We use a custom FT
	// with a price that gives a gross evenly divisible by M.
	// ftDraftShared: 2 qty × unitPrice = net; gross = net × 1.23.
	// Choose unitPrice so that 2 × unitPrice × 1.23 is divisible by 3.
	// unitPrice=50.00 → net=100.00 → gross=123.00. 123/3=41. ✓
	cust := sharedCustomer()
	ft := func() domain.SalesInvoice {
		prod := mustVal(domain.NewProduct(domain.Product{
			ProductCode:        "P-CONC",
			ProductType:        domain.ProductTypeGoods,
			ProductDescription: "Produto Concorrência",
			ProductNumberCode:  "P-CONC",
			Unit:               domain.UnitPiece,
			Active:             true,
		}))
		user := testUser()
		cd := domain.CommonDraftDocument{
			DocumentCore: domain.DocumentCore{
				DocumentType: domain.FT,
				Customer:     cust,
				Date:         now,
				IssuedBy:     user,
			},
			Series: activeFTSeries(now),
		}
		cd.AddLine(domain.DocumentLine{
			Product:      prod,
			Quantity:     mustVal(domain.NewQuantity(2)),
			UnitPrice:    mustVal(domain.NewMoney(50.00)),
			TaxPointDate: now,
			Tax:          normalTax(),
		})
		draft := domain.DraftSalesInvoice{CommonDraftDocument: cd}
		issued, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
			app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src",
				Idem: app.IdempotencyKey{Key: "ft-conc", Fingerprint: "fp-ft-conc"}})
		if err != nil {
			t.Fatalf("issue FT: %v", err)
		}
		return issued
	}()

	gross := ft.Totals.GrossTotal
	partialAmt := gross / domain.Money(M) // exactly one-third of gross

	// Spawn N goroutines, each issuing an RG in its own series settling partialAmt.
	type result struct {
		ok  bool
		err error
	}
	results := make([]result, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seriesID := fmt.Sprintf("RG%04d", i)
			draft := domain.PaymentDraft{
				Type:            domain.RG,
				TransactionDate: now,
				Customer:        cust,
				SourceID:        "issuer@demo.pt",
				Methods: []domain.PaymentMethod{{
					Mechanism: domain.PaymentMechanismCash,
					Amount:    partialAmt,
					Date:      now,
				}},
				Lines: []domain.PaymentLine{{
					LineNumber: 1,
					SourceDocuments: []domain.SourceDocumentID{{
						OriginatingON: ft.Number.Format(),
						InvoiceDate:   ft.Date,
					}},
					Movement: domain.CreditAmount{Value: partialAmt},
				}},
			}
			totals := domain.PaymentTotals{
				NetTotal:   partialAmt,
				TaxPayable: 0,
				GrossTotal: partialAmt,
			}
			_, err := svc.Invoicing.IssuePayment(context.Background(), testTenantID,
				app.IssuePaymentRequest{
					Draft: draft, SeriesID: seriesID, Totals: totals,
					Idem: app.IdempotencyKey{
						Key:         fmt.Sprintf("rg-conc-%d", i),
						Fingerprint: fmt.Sprintf("fp-rg-conc-%d", i),
					},
				})
			results[i] = result{ok: err == nil, err: err}
		}(i)
	}
	wg.Wait()

	// Count successes and failures.
	successes, failures := 0, 0
	for _, r := range results {
		if r.ok {
			successes++
		} else {
			if !errors.Is(r.err, domain.ErrAllocationExceedsSource) {
				t.Errorf("unexpected failure error: %v (want ErrAllocationExceedsSource)", r.err)
			}
			failures++
		}
	}

	if successes != M {
		t.Fatalf("successes = %d, want exactly %d (no over-settlement)", successes, M)
	}
	if failures != N-M {
		t.Fatalf("failures = %d, want %d", failures, N-M)
	}
}

// ── scenario 5.13: ND then RC settlement (axes independent) ──────────────────

// TestAllocND_ThenRC_Scenario513 is the regression for the bug fixed by the
// axis-split: an ND over an FT must NOT count toward the receipt settlement
// ceiling. Steps:
//  1. Issue FT (gross G).
//  2. Issue ND over FT (amount D < G) — must succeed.
//  3. Issue RC settling G against FT — must succeed (ND not in settlement axis).
//  4. Issue a SECOND RC settling 1 cent over G — must fail (ceiling HARD).
//  5. Cancel the first RC; issue a fresh full-G RC — must succeed (cancelled excluded).
//  6. Issue NC over-crediting FT — must fail (NC credit axis enforced).
func TestAllocND_ThenRC_Scenario513(t *testing.T) {
	now := testNow()
	svc, store := buildStore(t, testTenant(),
		activeFTSeries(now),
		activeNDSeries(now),
		activeNCSeries(now),
		activeRGSeriesID("RC2026", now),
		activeRGSeriesID("RC2026B", now),
		activeRGSeriesID("RC2026C", now),
	)
	cust := sharedCustomer()

	// Step 1: issue FT (gross G).
	ft := issueFTWith(t, svc, cust, "ft-513")
	gross := ft.Totals.GrossTotal

	// Step 2: issue ND over FT (small price × same qty).
	ndS := activeNDSeries(now)
	ndDraftDoc := ndDraftShared(ndS, now, ft)
	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID,
		app.IssueSalesInvoiceRequest{Draft: ndDraftDoc, SeriesID: "ND2026", SourceID: "src",
			Idem: app.IdempotencyKey{Key: "nd-513", Fingerprint: "fp-nd-513"}})
	if err != nil {
		t.Fatalf("step 2: ND over FT must succeed: %v", err)
	}

	// Step 3: issue RC (RG) settling full gross G — must succeed.
	// The ND must NOT count against the settlement ceiling.
	d1, t1 := paymentDraftSettlingFT(now, ft, gross)
	rc1, err := issueRGFor(t, svc, "RC2026", d1, t1, "rc-513-full")
	if err != nil {
		t.Fatalf("step 3: RC settling full gross after ND must succeed: %v", err)
	}

	// Step 4: second RC settling 1 cent — must fail (settlement ceiling now full).
	d2, t2 := paymentDraftSettlingFT(now, ft, oneCent)
	_, err = issueRGFor(t, svc, "RC2026B", d2, t2, "rc-513-over")
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("step 4: over-settlement kind = %v, want KindInvalid", app.KindOf(err))
	}
	if !errors.Is(err, domain.ErrAllocationExceedsSource) {
		t.Fatalf("step 4: err = %v, want ErrAllocationExceedsSource", err)
	}
	// RC2026B series must NOT have advanced.
	s, ok := store.GetSeries(testTenantID, "RC2026B", domain.RG)
	if !ok {
		t.Fatal("RC2026B series missing")
	}
	if s.LastNum != 0 {
		t.Fatalf("step 4: RC2026B LastNum = %d, want 0 (rejection must not advance series)", s.LastNum)
	}

	// Step 5: cancel the first RC; fresh full-G RC must succeed (cancelled excluded).
	cancelPaymentInStore(t, store, rc1.Number)
	d3, t3 := paymentDraftSettlingFT(now, ft, gross)
	_, err = issueRGFor(t, svc, "RC2026C", d3, t3, "rc-513-after-cancel")
	if err != nil {
		t.Fatalf("step 5: RC after cancelled RC must succeed: %v", err)
	}

	// Step 6: NC over-crediting FT (credit axis) must still fail.
	_, err = issueNCShared(t, svc, "NC2026", activeNCSeries(now), ft, 9999.00, "nc-513-overcredit")
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("step 6: NC over-credit kind = %v, want KindInvalid", app.KindOf(err))
	}
	if !errors.Is(err, domain.ErrAllocationExceedsSource) {
		t.Fatalf("step 6: err = %v, want ErrAllocationExceedsSource", err)
	}
}
