package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/at"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

const (
	maxCommAttempts = 5
	baseCommBackoff = time.Minute
	maxCommBackoff  = 30 * time.Minute
)

// ATClientFactory builds the per-tenant AT clients (credentials vary per tenant;
// endpoints/cert are global). The null implementation returns NullClient.
type ATClientFactory interface {
	ForTenant(t Tenant) (at.SeriesClient, at.InvoiceClient, at.TransportClient, error)
}

// CommService drains the AT-communication outbox. The ticker loop that calls
// DrainOnce lives in the composition root; DrainOnce is the unit of work.
type CommService struct {
	tenants TenantStore
	uow     UnitOfWork
	queue   OutboxQueue
	clients ATClientFactory
	clock   Clock
}

func newCommService(d Deps) *CommService {
	return &CommService{
		tenants: d.Tenants,
		uow:     d.UoW,
		queue:   d.Queue,
		clients: d.ATClients,
		clock:   d.Clock,
	}
}

// DrainOnce claims up to limit due tasks and processes each. Per-document AT
// rejections are recorded on the task; infrastructure failures (queue writes)
// are joined into the returned error.
func (s *CommService) DrainOnce(ctx context.Context, limit int) (int, error) {
	tasks, err := s.queue.ClaimDue(s.clock.Now(), limit)
	if err != nil {
		return 0, err
	}
	var errs []error
	for _, task := range tasks {
		if perr := s.process(ctx, task); perr != nil {
			errs = append(errs, fmt.Errorf("task %s: %w", task.ID, perr))
		}
	}
	return len(tasks), errors.Join(errs...)
}

func (s *CommService) process(ctx context.Context, task Task) error {
	tenant, err := s.tenants.Resolve(ctx, task.TenantID)
	if err != nil {
		// Likely an infrastructure blip, not a bad document — reschedule.
		return s.transient(task, fmt.Errorf("resolve tenant: %w", err))
	}
	_, invoices, _, err := s.clients.ForTenant(tenant)
	if err != nil {
		return s.transient(task, fmt.Errorf("at client: %w", err))
	}

	switch task.Kind {
	case KindInvoiceComm:
		var inv domain.SalesInvoice
		lerr := s.uow.Run(ctx, task.TenantID, func(tx RepoSet) error {
			var gerr error
			inv, gerr = tx.Documents().GetSalesInvoice(task.Number)
			return gerr
		})
		if lerr != nil {
			return s.transient(task, fmt.Errorf("load %s: %w", task.Number.Format(), lerr))
		}
		res, cerr := invoices.CommunicateInvoice(ctx, tenant.Company, inv)
		if cerr != nil {
			return s.classify(task, cerr)
		}
		return s.queue.Complete(task.ID, fmt.Sprintf("code=%d %s", res.Code, res.Message))
	default:
		// Transport/GT communication needs stock-movement issuance (a later plan).
		return s.terminal(task, fmt.Errorf("unsupported task kind %q", task.Kind))
	}
}

// classify routes an AT error: ambiguous outcomes are terminal but labelled
// for manual reconciliation; retryable ones reschedule with backoff; the rest
// are terminal.
func (s *CommService) classify(task Task, err error) error {
	if atErr, ok := errors.AsType[at.Error](err); ok {
		if atErr.Ambiguous {
			return s.terminal(task, fmt.Errorf("AMBIGUOUS OUTCOME — verify document state at AT before re-sending: %w", err))
		}
		if atErr.IsRetryable() && task.Attempts < maxCommAttempts {
			return s.scheduleRetry(task, err)
		}
	}
	return s.terminal(task, err)
}

// transient reschedules infrastructure failures with the same backoff/cap as
// retryable AT errors.
func (s *CommService) transient(task Task, err error) error {
	if task.Attempts < maxCommAttempts {
		return s.scheduleRetry(task, err)
	}
	return s.queue.Fail(task.ID, err.Error(), nil)
}

func (s *CommService) terminal(task Task, err error) error {
	return s.queue.Fail(task.ID, err.Error(), nil)
}

// scheduleRetry reschedules a task with the standard per-attempt backoff.
func (s *CommService) scheduleRetry(task Task, err error) error {
	retryAt := s.clock.Now().Add(commBackoff(task.Attempts))
	return s.queue.Fail(task.ID, err.Error(), &retryAt)
}

// commBackoff doubles from baseCommBackoff per claimed attempt (attempts >= 1),
// capped at maxCommBackoff.
func commBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return baseCommBackoff
	}
	d := baseCommBackoff << (attempts - 1)
	if d <= 0 || d > maxCommBackoff {
		return maxCommBackoff
	}
	return d
}
