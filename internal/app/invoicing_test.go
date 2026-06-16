package app_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func TestIssueSalesInvoice_HappyPath(t *testing.T) {
	svc, store := newFixture()
	draft := ftDraft(activeFTSeries(testNow()), testNow())

	doc, err := svc.Invoicing.IssueSalesInvoice(
		context.Background(), testTenantID, app.IssueSalesInvoiceRequest{
			Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"},
		},
	)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if doc.Number.Seq != 1 {
		t.Fatalf("seq = %d, want 1", doc.Number.Seq)
	}
	if store.SalesCount() != 1 {
		t.Fatalf("sales persisted = %d, want 1", store.SalesCount())
	}
	if store.OutboxLen() != 1 {
		t.Fatalf("outbox = %d, want 1 (CommRealtime enqueues)", store.OutboxLen())
	}
	s, ok := store.GetSeries(testTenantID, "FT2026", domain.FT)
	if !ok || s.LastNum != 1 {
		t.Fatalf("series LastNum = %d (ok=%v), want 1", s.LastNum, ok)
	}
}

func TestIssueSalesInvoice_IdempotentReplay(t *testing.T) {
	svc, store := newFixture()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	idem := app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}

	first, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: idem})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: idem})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.Number != second.Number {
		t.Fatalf("replay returned %s, want %s", second.Number.Format(), first.Number.Format())
	}
	if store.SalesCount() != 1 {
		t.Fatalf("sales = %d, want 1 (replay must not issue again)", store.SalesCount())
	}
}

func TestIssueSalesInvoice_IdempotencyMismatch(t *testing.T) {
	svc, _ := newFixture()
	draft := ftDraft(activeFTSeries(testNow()), testNow())

	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err = svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "DIFFERENT"}})
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", app.KindOf(err))
	}
	if !errors.Is(err, app.ErrIdempotencyMismatch) {
		t.Fatalf("err = %v, want ErrIdempotencyMismatch", err)
	}
}

func TestIssueSalesInvoice_RetriesOnVersionConflict(t *testing.T) {
	svc, store := newFixture()
	store.FailSeriesSaveOnce() // attempt 1's Series.Save returns ErrVersionConflict

	draft := ftDraft(activeFTSeries(testNow()), testNow())
	doc, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}})
	if err != nil {
		t.Fatalf("should succeed on retry: %v", err)
	}
	if doc.Number.Seq != 1 {
		t.Fatalf("seq = %d, want 1", doc.Number.Seq)
	}
	if doc.Totals.GrossTotal <= 0 {
		t.Fatalf("retried document GrossTotal = %d, want > 0 (totals corrupted)", doc.Totals.GrossTotal)
	}
	// Both attempts re-issue at Seq 1 (same document key), so SalesCount==1 here
	// asserts convergence to one document, not rollback itself — the snapshot/restore
	// of a document write is proven directly by memstore's
	// TestRun_RestoresSalesInvoiceOnError.
	if store.SalesCount() != 1 {
		t.Fatalf("sales = %d, want exactly 1 (converged to a single document)", store.SalesCount())
	}
	if store.OutboxLen() != 1 {
		t.Fatalf("outbox = %d, want exactly 1 comm task", store.OutboxLen())
	}
}

func TestIssueSalesInvoice_TenantIsolation(t *testing.T) {
	now := testNow()
	store := memstore.New()
	store.SeedSeries("tenant-a", activeFTSeries(now))
	store.SeedSeries("tenant-b", activeFTSeries(now))
	svc := app.New(app.Deps{
		Tenants: mapTenantStore{tenants: map[string]app.Tenant{
			"tenant-a": testTenantNamed("tenant-a"),
			"tenant-b": testTenantNamed("tenant-b"),
		}},
		UoW:      store,
		Clock:    fixedClock{t: now},
		Signer:   stubSigner{},
		Software: testSoftware(),
	})

	// Both tenants issue against series "FT2026" using the SAME idempotency key.
	// Tenant-keying must keep series, documents, and idempotency records separate.
	for _, tid := range []string{"tenant-a", "tenant-b"} {
		draft := ftDraft(activeFTSeries(now), now)
		if _, err := svc.Invoicing.IssueSalesInvoice(context.Background(), tid, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp"}}); err != nil {
			t.Fatalf("%s: %v", tid, err)
		}
	}

	a, _ := store.GetSeries("tenant-a", "FT2026", domain.FT)
	b, _ := store.GetSeries("tenant-b", "FT2026", domain.FT)
	if a.LastNum != 1 || b.LastNum != 1 {
		t.Fatalf("series not isolated: a=%d b=%d, want 1 and 1", a.LastNum, b.LastNum)
	}
	if store.SalesCount() != 2 {
		t.Fatalf("sales = %d, want 2 (one per tenant, identical DocNumber)", store.SalesCount())
	}
}

func TestIssueSalesInvoice_InvalidDraft(t *testing.T) {
	svc, _ := newFixture()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	draft.Lines = nil // domain validation rejects with ErrNoLines

	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}})
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", app.KindOf(err))
	}
}

func TestIssueSalesInvoice_SeriesNotIssuable(t *testing.T) {
	store := memstore.New()
	// Seed an UNREGISTERED series → CanIssue() == false.
	store.SeedSeries(testTenantID, mustVal(domain.NewSeries("FT2026", domain.FT)))
	svc := app.New(app.Deps{
		Tenants:  oneTenant(testTenant()),
		UoW:      store,
		Clock:    fixedClock{t: testNow()},
		Signer:   stubSigner{},
		Software: testSoftware(),
	})
	draft := ftDraft(mustVal(domain.NewSeries("FT2026", domain.FT)), testNow())

	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}})
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", app.KindOf(err))
	}
	if !errors.Is(err, app.ErrSeriesNotIssuable) {
		t.Fatalf("err = %v, want ErrSeriesNotIssuable", err)
	}
}

func TestIssueSalesInvoice_MonthlySAFTNoEnqueue(t *testing.T) {
	store := memstore.New()
	store.SeedSeries(testTenantID, activeFTSeries(testNow()))
	tn := testTenant()
	tn.CommMode = app.CommMonthlySAFT
	svc := app.New(app.Deps{
		Tenants:  oneTenant(tn),
		UoW:      store,
		Clock:    fixedClock{t: testNow()},
		Signer:   stubSigner{},
		Software: testSoftware(),
	})
	draft := ftDraft(activeFTSeries(testNow()), testNow())

	_, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if store.OutboxLen() != 0 {
		t.Fatalf("outbox = %d, want 0 for CommMonthlySAFT", store.OutboxLen())
	}
}

func TestIssueSalesInvoice_ConcurrentSameSeries(t *testing.T) {
	svc, store := newFixture()
	const N = 25

	var wg sync.WaitGroup
	seqs := make([]int, N)
	hashes := make([]domain.Hash, N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			draft := ftDraft(activeFTSeries(testNow()), testNow())
			key := app.IdempotencyKey{Key: fmt.Sprintf("k-%d", i), Fingerprint: "fp"}
			doc, err := svc.Invoicing.IssueSalesInvoice(
				context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: fmt.Sprintf("src-%d", i), Idem: key},
			)
			errs[i] = err
			if err == nil {
				seqs[i] = doc.Number.Seq
				hashes[i] = doc.Hash
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	if store.SalesCount() != N {
		t.Fatalf("docs = %d, want %d", store.SalesCount(), N)
	}

	// Sequence numbers must be exactly 1..N (no gaps, no duplicates).
	order := make([]int, N)
	copy(order, seqs)
	sort.Ints(order)
	for i := 0; i < N; i++ {
		if order[i] != i+1 {
			t.Fatalf("sequence not contiguous 1..%d: got %v", N, order)
		}
	}

	// Hash chain intact: the series head must equal the highest-seq document's hash.
	maxSeq, maxHash := 0, domain.Hash("")
	for i := 0; i < N; i++ {
		if seqs[i] > maxSeq {
			maxSeq, maxHash = seqs[i], hashes[i]
		}
	}
	s, _ := store.GetSeries(testTenantID, "FT2026", domain.FT)
	if s.LastNum != N {
		t.Fatalf("series LastNum = %d, want %d", s.LastNum, N)
	}
	if s.LastHash != string(maxHash) {
		t.Fatalf("series LastHash != last document hash — chain broken")
	}
}
