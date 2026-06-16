package memstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func mustSeries() domain.Series {
	s, err := domain.NewSeries("FT2026", domain.FT)
	if err != nil {
		panic(err)
	}
	return s
}

func TestRun_CommitsOnSuccess(t *testing.T) {
	s := New()
	if err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		return tx.Idempotency().Put(app.IdempotencyRecord{Key: "k"})
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		if _, gerr := tx.Idempotency().Get("k"); gerr != nil {
			t.Fatalf("committed record missing: %v", gerr)
		}
		return nil
	})
}

func TestRun_RestoresOnError(t *testing.T) {
	s := New()
	sentinel := errors.New("boom")
	err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		_ = tx.Idempotency().Put(app.IdempotencyRecord{Key: "k"})
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	_ = s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		if _, gerr := tx.Idempotency().Get("k"); !errors.Is(gerr, app.ErrNotFound) {
			t.Fatalf("rolled-back record still present: %v", gerr)
		}
		return nil
	})
}

func TestSeriesSave_VersionConflict(t *testing.T) {
	s := New()
	series := mustSeries()
	s.SeedSeries("t", series) // stored Version 0 under tenant "t"
	err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		return tx.Series().Save(series, 99) // expected 99 != stored 0
	})
	if !errors.Is(err, app.ErrVersionConflict) {
		t.Fatalf("err = %v, want ErrVersionConflict", err)
	}
}

func TestSeriesSave_InjectedConflictOnce(t *testing.T) {
	s := New()
	series := mustSeries()
	s.SeedSeries("t", series)
	s.FailSeriesSaveOnce()
	err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		return tx.Series().Save(series, 0)
	})
	if !errors.Is(err, app.ErrVersionConflict) {
		t.Fatalf("first save err = %v, want ErrVersionConflict", err)
	}
	if err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		return tx.Series().Save(series, 0)
	}); err != nil {
		t.Fatalf("second save err = %v, want nil", err)
	}
}

func TestSeries_TenantIsolation(t *testing.T) {
	s := New()
	s.SeedSeries("a", mustSeries())
	if _, ok := s.GetSeries("b", "FT2026", domain.FT); ok {
		t.Fatal("tenant b must not see tenant a's series")
	}
	if _, ok := s.GetSeries("a", "FT2026", domain.FT); !ok {
		t.Fatal("tenant a must see its own series")
	}
}

func TestRun_RestoresSalesInvoiceOnError(t *testing.T) {
	s := New()
	num, nerr := domain.NewDocNumber(domain.FT, "FT2026", 1)
	if nerr != nil {
		t.Fatal(nerr)
	}
	inv := domain.SalesInvoice{}
	inv.Number = num

	sentinel := errors.New("boom")
	err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		if serr := tx.Documents().SaveSalesInvoice(inv); serr != nil {
			return serr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	_ = s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		if _, gerr := tx.Documents().GetSalesInvoice(num); !errors.Is(gerr, app.ErrNotFound) {
			t.Fatalf("rolled-back sales invoice still present: %v", gerr)
		}
		return nil
	})
}

// TestRun_RestoresNewFamiliesOnError guards the snapshot/restore of the work,
// stock, and payment maps — a dropped or swapped map in Run's restore line would
// leave a committed write after a failed transaction.
func TestRun_RestoresNewFamiliesOnError(t *testing.T) {
	s := New()
	wnum := mustVal2(domain.NewDocNumber(domain.NE, "NE2026", 1))
	snum := mustVal2(domain.NewDocNumber(domain.GR, "GR2026", 1))
	pnum := mustVal2(domain.NewDocNumber(domain.RG, "RG2026", 1))

	var work domain.WorkDocument
	work.Number = wnum
	var stock domain.StockMovement
	stock.Number = snum
	pay := domain.Payment{Number: pnum}

	sentinel := errors.New("boom")
	err := s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		if e := tx.Documents().SaveWorkDocument(work); e != nil {
			return e
		}
		if e := tx.Documents().SaveStockMovement(stock); e != nil {
			return e
		}
		if e := tx.Documents().SavePayment(pay); e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	_ = s.Run(context.Background(), "t", func(tx app.RepoSet) error {
		if _, e := tx.Documents().GetWorkDocument(wnum); !errors.Is(e, app.ErrNotFound) {
			t.Fatalf("rolled-back work document still present: %v", e)
		}
		if _, e := tx.Documents().GetStockMovement(snum); !errors.Is(e, app.ErrNotFound) {
			t.Fatalf("rolled-back stock movement still present: %v", e)
		}
		if _, e := tx.Documents().GetPayment(pnum); !errors.Is(e, app.ErrNotFound) {
			t.Fatalf("rolled-back payment still present: %v", e)
		}
		return nil
	})
}

func enqueue(t *testing.T, s *Store, tenant string) app.Task {
	t.Helper()
	var got app.Task
	err := s.Run(context.Background(), tenant, func(tx app.RepoSet) error {
		return tx.Outbox().Enqueue(app.Task{TenantID: tenant, Kind: app.KindInvoiceComm,
			Number: mustVal2(domain.NewDocNumber(domain.FT, "FT2026", 1))})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range s.OutboxTasks() {
		got = task
	}
	return got
}

func mustVal2[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestEnqueue_AssignsIDAndPending(t *testing.T) {
	s := New()
	task := enqueue(t, s, "ten")
	if task.ID == "" {
		t.Fatal("enqueue must assign a non-empty ID")
	}
	if task.Status != app.TaskPending {
		t.Fatalf("status = %q, want pending", task.Status)
	}
}

func TestClaimDue_ClaimsPendingAndIncrementsAttempts(t *testing.T) {
	s := New()
	enqueue(t, s, "ten")
	claimed, err := s.ClaimDue(time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Status != app.TaskClaimed || claimed[0].Attempts != 1 {
		t.Fatalf("claimed = %+v", claimed)
	}
	again, _ := s.ClaimDue(time.Now(), 10)
	if len(again) != 0 {
		t.Fatalf("re-claim returned %d, want 0", len(again))
	}
}

func TestComplete_MarksDone(t *testing.T) {
	s := New()
	task := enqueue(t, s, "ten")
	_, _ = s.ClaimDue(time.Now(), 10)
	if err := s.Complete(task.ID, "code=0 ok"); err != nil {
		t.Fatal(err)
	}
	for _, x := range s.OutboxTasks() {
		if x.ID == task.ID && (x.Status != app.TaskDone || x.ATResult != "code=0 ok") {
			t.Fatalf("task = %+v", x)
		}
	}
}

func TestFail_RetryReschedulesPendingAndTerminal(t *testing.T) {
	s := New()
	task := enqueue(t, s, "ten")
	_, _ = s.ClaimDue(time.Now(), 10)
	retryAt := time.Now().Add(time.Hour)
	if err := s.Fail(task.ID, "boom", &retryAt); err != nil {
		t.Fatal(err)
	}
	for _, x := range s.OutboxTasks() {
		if x.ID == task.ID {
			if x.Status != app.TaskPending || !x.NextAttempt.Equal(retryAt) || x.LastError != "boom" {
				t.Fatalf("after retry-fail: %+v", x)
			}
		}
	}
	if err := s.Fail(task.ID, "dead", nil); err != nil {
		t.Fatal(err)
	}
	for _, x := range s.OutboxTasks() {
		if x.ID == task.ID && x.Status != app.TaskFailed {
			t.Fatalf("after terminal-fail: %+v", x)
		}
	}
}

func TestSeriesCreate_DuplicateConflicts(t *testing.T) {
	s := New()
	if err := s.Run(context.Background(), "ten", func(tx app.RepoSet) error {
		return tx.Series().Create(mustSeries())
	}); err != nil {
		t.Fatal(err)
	}
	err := s.Run(context.Background(), "ten", func(tx app.RepoSet) error {
		return tx.Series().Create(mustSeries())
	})
	if !errors.Is(err, app.ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestSalesInPeriod_FiltersByDate(t *testing.T) {
	s := New()
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	may := time.Date(2026, 5, 22, 9, 0, 0, 0, lisbon)
	jun := time.Date(2026, 6, 22, 9, 0, 0, 0, lisbon)
	mk := func(seq int, when time.Time) domain.SalesInvoice {
		inv := domain.SalesInvoice{}
		inv.Number = mustVal2(domain.NewDocNumber(domain.FT, "FT2026", seq))
		inv.Date = when
		return inv
	}
	if err := s.Run(context.Background(), "ten", func(tx app.RepoSet) error {
		_ = tx.Documents().SaveSalesInvoice(mk(1, may))
		_ = tx.Documents().SaveSalesInvoice(mk(2, jun))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var got []domain.SalesInvoice
	_ = s.Run(context.Background(), "ten", func(tx app.RepoSet) error {
		var gerr error
		got, gerr = tx.Documents().SalesInPeriod(
			time.Date(2026, 5, 1, 0, 0, 0, 0, lisbon),
			time.Date(2026, 5, 31, 23, 59, 59, 0, lisbon))
		return gerr
	})
	if len(got) != 1 || got[0].Number.Seq != 1 {
		t.Fatalf("got %d invoices %v, want exactly seq 1", len(got), got)
	}
}

// TestLiveRectifyingNotes pins the memstore scan that the V1 cancel guard uses.
func TestLiveRectifyingNotes(t *testing.T) {
	ctx := context.Background()
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	now := time.Date(2026, 5, 22, 9, 0, 0, 0, lisbon)

	ftNum := mustVal2(domain.NewDocNumber(domain.FT, "FT2026", 1))
	ncNum := mustVal2(domain.NewDocNumber(domain.NC, "NC2026", 1))
	ndNum := mustVal2(domain.NewDocNumber(domain.ND, "ND2026", 1))

	// helper: build a minimal SalesInvoice with a given NC/ND referencing target
	makeNC := func(n domain.DocNumber, target string, cancelled bool) domain.SalesInvoice {
		si := domain.SalesInvoice{}
		si.Number = n
		si.DocumentType = n.Type
		si.Date = now
		si.Lines = []domain.DocumentLine{{
			LineNumber: 1,
			References: []domain.DocReference{{Reference: target}},
		}}
		if cancelled {
			si.Status = domain.StatusCancelled
		} else {
			si.Status = domain.StatusNormal
		}
		return si
	}

	t.Run("live_NC_referencing_FT_returned", func(t *testing.T) {
		s := New()
		ft := domain.SalesInvoice{}
		ft.Number = ftNum
		ft.DocumentType = domain.FT
		ft.Status = domain.StatusNormal

		nc := makeNC(ncNum, ftNum.Format(), false)

		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			_ = tx.Documents().SaveSalesInvoice(ft)
			return tx.Documents().SaveSalesInvoice(nc)
		})

		var got []domain.SalesInvoice
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			got, err = tx.Documents().LiveRectifyingNotes(ftNum)
			return err
		})
		if len(got) != 1 || got[0].Number.Seq != 1 {
			t.Fatalf("got %d results, want 1 NC", len(got))
		}
	})

	t.Run("cancelled_NC_excluded", func(t *testing.T) {
		s := New()
		nc := makeNC(ncNum, ftNum.Format(), true /* cancelled */)
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			return tx.Documents().SaveSalesInvoice(nc)
		})
		var got []domain.SalesInvoice
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			got, err = tx.Documents().LiveRectifyingNotes(ftNum)
			return err
		})
		if len(got) != 0 {
			t.Fatalf("cancelled NC must not appear, got %d", len(got))
		}
	})

	t.Run("live_ND_included", func(t *testing.T) {
		s := New()
		nd := makeNC(ndNum, ftNum.Format(), false)
		nd.DocumentType = domain.ND
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			return tx.Documents().SaveSalesInvoice(nd)
		})
		var got []domain.SalesInvoice
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			got, err = tx.Documents().LiveRectifyingNotes(ftNum)
			return err
		})
		if len(got) != 1 {
			t.Fatalf("live ND must appear, got %d", len(got))
		}
	})

	t.Run("FT_referencing_FT_excluded", func(t *testing.T) {
		s := New()
		// An FT whose line references the target (edge case: not NC/ND)
		ft2Num := mustVal2(domain.NewDocNumber(domain.FT, "FT2026", 2))
		ft2 := makeNC(ft2Num, ftNum.Format(), false)
		ft2.DocumentType = domain.FT
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			return tx.Documents().SaveSalesInvoice(ft2)
		})
		var got []domain.SalesInvoice
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			got, err = tx.Documents().LiveRectifyingNotes(ftNum)
			return err
		})
		if len(got) != 0 {
			t.Fatalf("FT must not appear even if it references target, got %d", len(got))
		}
	})

	t.Run("unparseable_ref_no_panic", func(t *testing.T) {
		s := New()
		si := domain.SalesInvoice{}
		si.Number = ncNum
		si.DocumentType = domain.NC
		si.Status = domain.StatusNormal
		si.Lines = []domain.DocumentLine{{
			LineNumber: 1,
			References: []domain.DocReference{{Reference: "not-a-doc-number"}},
		}}
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			return tx.Documents().SaveSalesInvoice(si)
		})
		var got []domain.SalesInvoice
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			got, err = tx.Documents().LiveRectifyingNotes(ftNum)
			return err
		})
		if len(got) != 0 {
			t.Fatalf("unparseable ref must not match, got %d", len(got))
		}
	})

	t.Run("tenant_isolation", func(t *testing.T) {
		s := New()
		nc := makeNC(ncNum, ftNum.Format(), false)
		// Store the NC under tenant "a"
		_ = s.Run(ctx, "a", func(tx app.RepoSet) error {
			return tx.Documents().SaveSalesInvoice(nc)
		})
		// Query from tenant "b" — must see nothing
		var got []domain.SalesInvoice
		_ = s.Run(ctx, "b", func(tx app.RepoSet) error {
			var err error
			got, err = tx.Documents().LiveRectifyingNotes(ftNum)
			return err
		})
		if len(got) != 0 {
			t.Fatalf("tenant b must not see tenant a's NC, got %d", len(got))
		}
	})
}

func TestOutboxFind_ByTenantAndNumber(t *testing.T) {
	s := New()
	num := mustVal2(domain.NewDocNumber(domain.FT, "FT2026", 1))
	if err := s.Run(context.Background(), "ten", func(tx app.RepoSet) error {
		return tx.Outbox().Enqueue(app.Task{TenantID: "ten", Kind: app.KindInvoiceComm, Number: num})
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Find("ten", num); !ok {
		t.Fatal("task not found for its tenant+number")
	}
	if _, ok, _ := s.Find("other", num); ok {
		t.Fatal("task must not be found for a different tenant")
	}
}

// TestSourceState_ConsumedAgainst pins the B3-a SourceState + consumedAgainst logic.
// Since the fix the two axes are separate:
//
//	FT gross 100 (€100.00 = 10_000_000)
//	NC referencing FT, line gross 40 (€40.00 = 4_000_000)
//	cancelled NC referencing FT → excluded from AllocCredit axis
//	RG settling FT for 30 (€30.00 = 3_000_000)
//	AllocCredit  axis: Consumed == 4_000_000  (NC only, cancelled NC excluded)
//	AllocSettlement axis: Consumed == 3_000_000  (RG only)
//	unknown number → ErrNotFound
func TestSourceState_ConsumedAgainst(t *testing.T) {
	ctx := context.Background()

	ftNum := mustVal2(domain.NewDocNumber(domain.FT, "FT2026", 1))
	ncNum := mustVal2(domain.NewDocNumber(domain.NC, "NC2026", 1))
	ncCancelledNum := mustVal2(domain.NewDocNumber(domain.NC, "NC2026", 2))
	rgNum := mustVal2(domain.NewDocNumber(domain.RG, "RG2026", 1))
	unknownNum := mustVal2(domain.NewDocNumber(domain.FT, "FT2026", 99))

	// FT: gross €100.00 (raw 10_000_000 = 100 × 100_000 internal units)
	ft := domain.SalesInvoice{}
	ft.Number = ftNum
	ft.DocumentType = domain.FT
	ft.Status = domain.StatusNormal
	ft.Totals.GrossTotal = domain.Money(10_000_000)

	// NC referencing FT: line UnitPrice €40.00 × qty 1, no tax → LineTotal = 4_000_000
	nc := domain.SalesInvoice{}
	nc.Number = ncNum
	nc.DocumentType = domain.NC
	nc.Status = domain.StatusNormal
	nc.Lines = []domain.DocumentLine{{
		LineNumber: 1,
		UnitPrice:  domain.Money(4_000_000),  // €40.00
		Quantity:   domain.Quantity(100_000), // 1.00 unit at scale
		References: []domain.DocReference{{Reference: ftNum.Format()}},
	}}

	// Cancelled NC referencing FT: must be excluded from Consumed
	ncCancelled := domain.SalesInvoice{}
	ncCancelled.Number = ncCancelledNum
	ncCancelled.DocumentType = domain.NC
	ncCancelled.Status = domain.StatusCancelled
	ncCancelled.Lines = []domain.DocumentLine{{
		LineNumber: 1,
		UnitPrice:  domain.Money(2_000_000), // €20.00 — must not count
		Quantity:   domain.Quantity(100_000),
		References: []domain.DocReference{{Reference: ftNum.Format()}},
	}}

	// RG settling FT for €30.00 (raw 3_000_000)
	rg := domain.Payment{}
	rg.Number = rgNum
	rg.Status = domain.StatusNormal
	rg.Lines = []domain.PaymentLine{{
		LineNumber: 1,
		Movement:   domain.CreditAmount{Value: domain.Money(3_000_000)}, // €30.00
		SourceDocuments: []domain.SourceDocumentID{{
			OriginatingON: ftNum.Format(),
			InvoiceDate:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		}},
	}}

	s := New()
	if err := s.Run(ctx, "t", func(tx app.RepoSet) error {
		if e := tx.Documents().SaveSalesInvoice(ft); e != nil {
			return e
		}
		if e := tx.Documents().SaveSalesInvoice(nc); e != nil {
			return e
		}
		if e := tx.Documents().SaveSalesInvoice(ncCancelled); e != nil {
			return e
		}
		return tx.Documents().SavePayment(rg)
	}); err != nil {
		t.Fatal(err)
	}

	// AllocCredit axis: NC gross 40 consumed (cancelled NC excluded); RG not counted.
	t.Run("credit_axis_NC_gross_40", func(t *testing.T) {
		var st domain.SourceDocState
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			st, err = tx.Documents().SourceState(ftNum, app.AllocCredit)
			return err
		})
		if st.Consumed != domain.Money(4_000_000) {
			t.Fatalf("AllocCredit Consumed = %d, want 4_000_000 (NC only)", st.Consumed)
		}
		if st.Gross != domain.Money(10_000_000) {
			t.Fatalf("Gross = %d, want 10_000_000", st.Gross)
		}
		if st.Status != domain.StatusNormal {
			t.Fatalf("Status = %q, want N", st.Status)
		}
	})

	// AllocSettlement axis: RG settle 30 consumed; NC not counted.
	t.Run("settlement_axis_RG_gross_30", func(t *testing.T) {
		var st domain.SourceDocState
		_ = s.Run(ctx, "t", func(tx app.RepoSet) error {
			var err error
			st, err = tx.Documents().SourceState(ftNum, app.AllocSettlement)
			return err
		})
		if st.Consumed != domain.Money(3_000_000) {
			t.Fatalf("AllocSettlement Consumed = %d, want 3_000_000 (RG only)", st.Consumed)
		}
	})

	// Unknown number → ErrNotFound (axis is irrelevant for the not-found check)
	t.Run("unknown_number_ErrNotFound", func(t *testing.T) {
		err := s.Run(ctx, "t", func(tx app.RepoSet) error {
			_, gerr := tx.Documents().SourceState(unknownNum, app.AllocCredit)
			return gerr
		})
		if !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}
