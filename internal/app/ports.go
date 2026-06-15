package app

import (
	"context"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Clock is the time source; the domain stamps SystemEntryDate from Now().
type Clock interface {
	Now() time.Time
}

// UnitOfWork runs fn atomically with repos bound to one tenant. fn must be safe
// to re-execute: on ErrVersionConflict the caller re-runs the whole function.
type UnitOfWork interface {
	Run(ctx context.Context, tenantID string, fn func(tx RepoSet) error) error
}

// RepoSet is the tenant-bound repository handle handed to a UnitOfWork callback.
// Implementations MUST scope every operation to the bound tenant.
type RepoSet interface {
	Series() SeriesRepo
	Documents() DocumentRepo
	Outbox() OutboxRepo
	Idempotency() IdempotencyRepo
}

type SeriesRepo interface {
	Get(id string, dt domain.DocumentType) (domain.Series, error)
	// Save returns ErrVersionConflict if the stored Version != expectedVersion.
	Save(s domain.Series, expectedVersion uint64) error
	// Create inserts a new series; returns ErrAlreadyExists if one with the same
	// (tenant, id, doc type) is already stored.
	Create(s domain.Series) error
}

// DocumentRepo holds the document-side operations the issuance spine needs.
// Each family exposes the same trio: Save (persist an issued document), Get
// (load one by number, for idempotent replay and cross-document readers), and
// *InPeriod (the period reader the SAF-T export consumes). Sales/work/stock are
// placed by DocumentCore.Date; payments by TransactionDate.
type DocumentRepo interface {
	SaveSalesInvoice(domain.SalesInvoice) error
	GetSalesInvoice(domain.DocNumber) (domain.SalesInvoice, error)
	// SalesInPeriod returns this tenant's sales invoices whose Date falls within
	// [from, to], ordered by sequence number.
	SalesInPeriod(from, to time.Time) ([]domain.SalesInvoice, error)

	SaveWorkDocument(domain.WorkDocument) error
	GetWorkDocument(domain.DocNumber) (domain.WorkDocument, error)
	WorkInPeriod(from, to time.Time) ([]domain.WorkDocument, error)

	SaveStockMovement(domain.StockMovement) error
	GetStockMovement(domain.DocNumber) (domain.StockMovement, error)
	StockInPeriod(from, to time.Time) ([]domain.StockMovement, error)

	SavePayment(domain.Payment) error
	GetPayment(domain.DocNumber) (domain.Payment, error)
	// PaymentsInPeriod selects by TransactionDate (payments carry no DocumentCore.Date).
	PaymentsInPeriod(from, to time.Time) ([]domain.Payment, error)
}

// OutboxRepo is the enqueue side of the AT-communication outbox — transactional
// and tenant-bound. The cross-tenant worker-side queue arrives in a later plan.
type OutboxRepo interface {
	Enqueue(Task) error
}

type IdempotencyRepo interface {
	Get(key string) (IdempotencyRecord, error)
	Put(IdempotencyRecord) error
}

// TaskKind identifies which AT webservice an outbox task targets.
type TaskKind string

const (
	KindInvoiceComm   TaskKind = "invoice"
	KindTransportComm TaskKind = "transport"
)

// TaskStatus is the lifecycle state of an outbox task.
type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskClaimed TaskStatus = "claimed"
	TaskDone    TaskStatus = "done"
	TaskFailed  TaskStatus = "failed"
)

// Task is an AT-communication work item. Issuance enqueues it with TenantID/Kind/
// Number set; the store assigns ID and Status=Pending. The worker drives the rest.
type Task struct {
	ID          string
	TenantID    string
	Kind        TaskKind
	Number      domain.DocNumber
	Status      TaskStatus
	Attempts    int
	NextAttempt time.Time // zero = immediately due
	LastError   string
	ATResult    string
}

// OutboxQueue is the worker-side, cross-tenant port (outside UnitOfWork).
// Enqueue stays on the tenant-bound OutboxRepo; draining is operational.
type OutboxQueue interface {
	// ClaimDue atomically marks up to limit Pending tasks whose NextAttempt <= now
	// as Claimed (incrementing Attempts) and returns copies of them.
	ClaimDue(now time.Time, limit int) ([]Task, error)
	// Complete marks a task Done and records its AT result.
	Complete(id, atResult string) error
	// Fail records the error; non-nil retryAt reschedules to Pending at that time,
	// nil makes it terminal Failed.
	Fail(id, errMsg string, retryAt *time.Time) error
	// Find returns the outbox task for (tenant, document number), if any.
	Find(tenantID string, number domain.DocNumber) (Task, bool, error)
}

// IdempotencyRecord deduplicates issuance: Key → (request fingerprint, issued number).
type IdempotencyRecord struct {
	Key         string
	Fingerprint string
	DocNumber   domain.DocNumber
}
