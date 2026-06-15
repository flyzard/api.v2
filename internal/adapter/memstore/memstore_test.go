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
