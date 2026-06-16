package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/at"
	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// nullATFactory hands every tenant the in-memory AT NullClient.
type nullATFactory struct{ c *at.NullClient }

func (f nullATFactory) ForTenant(_ app.Tenant) (at.SeriesClient, at.InvoiceClient, at.TransportClient, error) {
	return f.c, f.c, f.c, nil
}

// newCommFixture wires issuance + a comm service over one store, with the null AT client.
func newCommFixture() (*app.Services, *memstore.Store) {
	now := testNow()
	store := memstore.New()
	store.SeedSeries(testTenantID, activeFTSeries(now))
	svc := app.New(app.Deps{
		Tenants:   oneTenant(testTenant()),
		UoW:       store,
		Queue:     store,
		Clock:     fixedClock{t: now},
		Signer:    stubSigner{},
		ATClients: nullATFactory{c: at.NewNullClient()},
		Software:  testSoftware(),
	})
	return svc, store
}

func TestDrainOnce_HappyPath(t *testing.T) {
	svc, store := newCommFixture()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	if _, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}}); err != nil {
		t.Fatalf("issue: %v", err)
	}

	n, err := svc.Comm.DrainOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 1 {
		t.Fatalf("drained %d, want 1", n)
	}
	tasks := store.OutboxTasks()
	if len(tasks) != 1 || tasks[0].Status != app.TaskDone {
		t.Fatalf("task = %+v, want Done", tasks)
	}
	if tasks[0].ATResult == "" {
		t.Fatal("expected ATResult recorded on success")
	}
}

// stubInvoiceClient returns a fixed result/error for every call.
type stubInvoiceClient struct {
	res *at.InvoiceResult
	err error
}

func (c stubInvoiceClient) CommunicateInvoice(_ context.Context, _ domain.Company, _ domain.SalesInvoice) (*at.InvoiceResult, error) {
	return c.res, c.err
}

// stubATFactory hands out a fixed invoice client (series/transport unused here).
type stubATFactory struct{ inv at.InvoiceClient }

func (f stubATFactory) ForTenant(_ app.Tenant) (at.SeriesClient, at.InvoiceClient, at.TransportClient, error) {
	return nil, f.inv, nil, nil
}

func commFixtureWith(inv at.InvoiceClient) (*app.Services, *memstore.Store) {
	now := testNow()
	store := memstore.New()
	store.SeedSeries(testTenantID, activeFTSeries(now))
	svc := app.New(app.Deps{
		Tenants:   oneTenant(testTenant()),
		UoW:       store,
		Queue:     store,
		Clock:     fixedClock{t: now},
		Signer:    stubSigner{},
		ATClients: stubATFactory{inv: inv},
		Software:  testSoftware(),
	})
	return svc, store
}

func issueForComm(t *testing.T, svc *app.Services) {
	t.Helper()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	if _, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, app.IssueSalesInvoiceRequest{Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}}); err != nil {
		t.Fatalf("issue: %v", err)
	}
}

func TestDrainOnce_RetryableReschedules(t *testing.T) {
	svc, store := commFixtureWith(stubInvoiceClient{err: at.Error{Code: "CONNECTION", Message: "timeout"}})
	issueForComm(t, svc)

	if _, err := svc.Comm.DrainOnce(context.Background(), 10); err != nil {
		t.Fatalf("drain: %v", err)
	}
	task := store.OutboxTasks()[0]
	if task.Status != app.TaskPending {
		t.Fatalf("status = %q, want pending (rescheduled)", task.Status)
	}
	if !task.NextAttempt.After(testNow()) {
		t.Fatalf("NextAttempt = %v, want after now (backoff)", task.NextAttempt)
	}
	if task.Attempts != 1 || task.LastError == "" {
		t.Fatalf("task = %+v, want attempts=1 + LastError", task)
	}
}

func TestDrainOnce_NonRetryableTerminal(t *testing.T) {
	svc, store := commFixtureWith(stubInvoiceClient{err: at.Error{Code: "3001", Message: "rejeitado"}})
	issueForComm(t, svc)

	if _, err := svc.Comm.DrainOnce(context.Background(), 10); err != nil {
		t.Fatalf("drain: %v", err)
	}
	task := store.OutboxTasks()[0]
	if task.Status != app.TaskFailed {
		t.Fatalf("status = %q, want failed (deterministic AT error)", task.Status)
	}
}

// movableClock is a Clock whose Now() can be advanced between drains.
type movableClock struct{ t time.Time }

func (c *movableClock) Now() time.Time { return c.t }

func TestDrainOnce_ExhaustsRetriesToTerminal(t *testing.T) {
	clk := &movableClock{t: testNow()}
	store := memstore.New()
	store.SeedSeries(testTenantID, activeFTSeries(testNow()))
	svc := app.New(app.Deps{
		Tenants:   oneTenant(testTenant()),
		UoW:       store,
		Queue:     store,
		Clock:     clk,
		Signer:    stubSigner{},
		ATClients: stubATFactory{inv: stubInvoiceClient{err: at.Error{Code: "CONNECTION", Message: "timeout"}}},
		Software:  testSoftware(),
	})
	issueForComm(t, svc)

	// maxCommAttempts is 5 (unexported); drain more than that, advancing the
	// clock past each backoff (max 30m) so the rescheduled task is due again.
	// A retryable failure on the attempt cap must become terminal Failed.
	for i := 0; i < 7; i++ {
		if _, err := svc.Comm.DrainOnce(context.Background(), 10); err != nil {
			t.Fatalf("drain %d: %v", i, err)
		}
		clk.t = clk.t.Add(time.Hour)
	}
	task := store.OutboxTasks()[0]
	if task.Status != app.TaskFailed {
		t.Fatalf("status = %q after exhausting retries, want failed", task.Status)
	}
}

func TestDrainOnce_AmbiguousIsTerminalAndLabelled(t *testing.T) {
	svc, store := commFixtureWith(stubInvoiceClient{
		err: at.Error{Code: "4001", Message: "Documento ja registado", Ambiguous: true},
	})
	issueForComm(t, svc)

	n, err := svc.Comm.DrainOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 1 {
		t.Fatalf("drained %d, want 1", n)
	}

	task := store.OutboxTasks()[0]
	if task.Status != app.TaskFailed {
		t.Fatalf("status = %q, want failed (ambiguous is terminal)", task.Status)
	}
	if !strings.Contains(task.LastError, "AMBIGUOUS") {
		t.Fatalf("LastError = %q, want it to contain AMBIGUOUS", task.LastError)
	}
}

func TestDrainOnce_QueueWriteErrorPropagates(t *testing.T) {
	svc, store := commFixtureWith(stubInvoiceClient{
		res: &at.InvoiceResult{Code: 200, Message: "ok"},
	})
	issueForComm(t, svc)

	store.FailNextComplete()

	_, err := svc.Comm.DrainOnce(context.Background(), 10)
	if err == nil {
		t.Fatal("expected DrainOnce to return error when Complete fails, got nil")
	}
}

func TestDrainOnce_TenantResolvableTransient(t *testing.T) {
	now := testNow()
	store := memstore.New()
	store.SeedSeries(testTenantID, activeFTSeries(now))
	// Use a tenant store that always fails Resolve.
	svc := app.New(app.Deps{
		Tenants:   mapTenantStore{tenants: map[string]app.Tenant{}}, // empty — Resolve returns ErrNotFound
		UoW:       store,
		Queue:     store,
		Clock:     fixedClock{t: now},
		Signer:    stubSigner{},
		ATClients: stubATFactory{inv: stubInvoiceClient{res: &at.InvoiceResult{Code: 200, Message: "ok"}}},
		Software:  testSoftware(),
	})

	// Seed a series + issue an invoice via a separate fixture so there's something to drain.
	issuer := app.New(app.Deps{
		Tenants:   oneTenant(testTenant()),
		UoW:       store,
		Queue:     store,
		Clock:     fixedClock{t: now},
		Signer:    stubSigner{},
		ATClients: nullATFactory{c: at.NewNullClient()},
		Software:  testSoftware(),
	})
	issueForComm(t, issuer)

	if _, err := svc.Comm.DrainOnce(context.Background(), 10); err != nil {
		t.Fatalf("drain: %v", err)
	}
	task := store.OutboxTasks()[0]
	// Tenant-resolve failure is transient (infrastructure blip), not terminal.
	if task.Status != app.TaskPending {
		t.Fatalf("status = %q, want pending (transient reschedule)", task.Status)
	}
	if task.LastError == "" {
		t.Fatal("expected LastError set on transient reschedule")
	}
}
