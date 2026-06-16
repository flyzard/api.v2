// Package memstore is an in-memory implementation of the application layer's
// persistence ports. Every map is keyed by tenant first, so repositories bound
// to one tenant (via UnitOfWork.Run) can never read or write another tenant's
// data. A single mutex serializes transactions; Run snapshots the maps before
// fn and restores them on error, giving honest all-or-nothing semantics.
// Because Run holds the lock for the whole transaction, the repository methods
// do not lock; the standalone accessor helpers (used outside Run) do.
package memstore

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

type Store struct {
	mu       sync.Mutex
	series   map[string]domain.Series
	sales    map[string]domain.SalesInvoice
	work     map[string]domain.WorkDocument
	stock    map[string]domain.StockMovement
	payments map[string]domain.Payment
	idem     map[string]app.IdempotencyRecord
	outbox   []app.Task

	nextTaskID int

	// failSeriesSaveOnce forces the next SeriesRepo.Save to return
	// ErrVersionConflict, for exercising the service retry path.
	failSeriesSaveOnce bool

	// failCompleteOnce forces the next OutboxQueue.Complete to return an error,
	// for exercising the DrainOnce error-propagation path.
	failCompleteOnce bool
}

func New() *Store {
	return &Store{
		series:   make(map[string]domain.Series),
		sales:    make(map[string]domain.SalesInvoice),
		work:     make(map[string]domain.WorkDocument),
		stock:    make(map[string]domain.StockMovement),
		payments: make(map[string]domain.Payment),
		idem:     make(map[string]app.IdempotencyRecord),
	}
}

// sep is the tenant/key delimiter — a control byte that cannot appear in a
// series ID, document number, or idempotency key.
const sep = "\x1f"

func seriesKey(tenantID, id string, dt domain.DocumentType) string {
	return tenantID + sep + string(dt) + sep + id
}

// docKey keys any family's issued document by tenant + formatted number. The
// number embeds the document type, so families never collide even in one map —
// but each family keeps its own map for clean snapshot/restore.
func docKey(tenantID string, n domain.DocNumber) string {
	return tenantID + sep + n.Format()
}
func idemStoreKey(tenantID, key string) string {
	return tenantID + sep + key
}

// Run holds the store lock for the whole transaction; repo methods below do not
// lock. On error it restores the pre-transaction snapshot.
func (s *Store) Run(ctx context.Context, tenantID string, fn func(app.RepoSet) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapSeries := maps.Clone(s.series)
	snapSales := maps.Clone(s.sales)
	snapWork := maps.Clone(s.work)
	snapStock := maps.Clone(s.stock)
	snapPayments := maps.Clone(s.payments)
	snapIdem := maps.Clone(s.idem)
	snapOutbox := slices.Clone(s.outbox)

	if err := fn(&repoSet{store: s, tenantID: tenantID}); err != nil {
		s.series, s.sales, s.idem, s.outbox = snapSeries, snapSales, snapIdem, snapOutbox
		s.work, s.stock, s.payments = snapWork, snapStock, snapPayments
		return err
	}
	return nil
}

// ── test / wiring helpers (acquire the lock; never call inside Run) ──────────

func (s *Store) SeedSeries(tenantID string, series domain.Series) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.series[seriesKey(tenantID, series.ID, series.DocType)] = series
}

func (s *Store) GetSeries(tenantID, id string, dt domain.DocumentType) (domain.Series, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.series[seriesKey(tenantID, id, dt)]
	return v, ok
}

func (s *Store) GetSalesInvoice(tenantID string, n domain.DocNumber) (domain.SalesInvoice, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sales[docKey(tenantID, n)]
	return v, ok
}

func (s *Store) SalesCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sales)
}

func (s *Store) WorkCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.work)
}

func (s *Store) StockCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.stock)
}

func (s *Store) PaymentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.payments)
}

func (s *Store) OutboxLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.outbox)
}

func (s *Store) FailSeriesSaveOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSeriesSaveOnce = true
}

func (s *Store) FailNextComplete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCompleteOnce = true
}

// ── repositories (tenant-bound; no locking — Run holds the lock) ─────────────

type repoSet struct {
	store    *Store
	tenantID string
}

func (r *repoSet) Series() app.SeriesRepo           { return seriesRepo{r.store, r.tenantID} }
func (r *repoSet) Documents() app.DocumentRepo      { return docRepo{r.store, r.tenantID} }
func (r *repoSet) Outbox() app.OutboxRepo           { return outboxRepo{r.store, r.tenantID} }
func (r *repoSet) Idempotency() app.IdempotencyRepo { return idemRepo{r.store, r.tenantID} }

type seriesRepo struct {
	store    *Store
	tenantID string
}

func (r seriesRepo) Get(id string, dt domain.DocumentType) (domain.Series, error) {
	v, ok := r.store.series[seriesKey(r.tenantID, id, dt)]
	if !ok {
		return domain.Series{}, app.ErrNotFound
	}
	return v, nil
}

func (r seriesRepo) Save(s domain.Series, expectedVersion uint64) error {
	if r.store.failSeriesSaveOnce {
		r.store.failSeriesSaveOnce = false
		return app.ErrVersionConflict
	}
	k := seriesKey(r.tenantID, s.ID, s.DocType)
	cur, ok := r.store.series[k]
	if ok && cur.Version != expectedVersion {
		return app.ErrVersionConflict
	}
	r.store.series[k] = s
	return nil
}

func (r seriesRepo) Create(s domain.Series) error {
	k := seriesKey(r.tenantID, s.ID, s.DocType)
	if _, ok := r.store.series[k]; ok {
		return app.ErrAlreadyExists
	}
	r.store.series[k] = s
	return nil
}

type docRepo struct {
	store    *Store
	tenantID string
}

func (r docRepo) SaveSalesInvoice(d domain.SalesInvoice) error {
	r.store.sales[docKey(r.tenantID, d.Number)] = d
	return nil
}

func (r docRepo) GetSalesInvoice(n domain.DocNumber) (domain.SalesInvoice, error) {
	v, ok := r.store.sales[docKey(r.tenantID, n)]
	if !ok {
		return domain.SalesInvoice{}, app.ErrNotFound
	}
	return v, nil
}

func (r docRepo) SalesInPeriod(from, to time.Time) ([]domain.SalesInvoice, error) {
	prefix := r.tenantID + sep
	var out []domain.SalesInvoice
	for k, v := range r.store.sales {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if v.Date.Before(from) || v.Date.After(to) {
			continue
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b domain.SalesInvoice) int { return a.Number.Seq - b.Number.Seq })
	return out, nil
}

func (r docRepo) SaveWorkDocument(d domain.WorkDocument) error {
	r.store.work[docKey(r.tenantID, d.Number)] = d
	return nil
}

func (r docRepo) GetWorkDocument(n domain.DocNumber) (domain.WorkDocument, error) {
	v, ok := r.store.work[docKey(r.tenantID, n)]
	if !ok {
		return domain.WorkDocument{}, app.ErrNotFound
	}
	return v, nil
}

func (r docRepo) WorkInPeriod(from, to time.Time) ([]domain.WorkDocument, error) {
	prefix := r.tenantID + sep
	var out []domain.WorkDocument
	for k, v := range r.store.work {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if v.Date.Before(from) || v.Date.After(to) {
			continue
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b domain.WorkDocument) int { return a.Number.Seq - b.Number.Seq })
	return out, nil
}

func (r docRepo) SaveStockMovement(d domain.StockMovement) error {
	r.store.stock[docKey(r.tenantID, d.Number)] = d
	return nil
}

func (r docRepo) GetStockMovement(n domain.DocNumber) (domain.StockMovement, error) {
	v, ok := r.store.stock[docKey(r.tenantID, n)]
	if !ok {
		return domain.StockMovement{}, app.ErrNotFound
	}
	return v, nil
}

func (r docRepo) StockInPeriod(from, to time.Time) ([]domain.StockMovement, error) {
	prefix := r.tenantID + sep
	var out []domain.StockMovement
	for k, v := range r.store.stock {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if v.Date.Before(from) || v.Date.After(to) {
			continue
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b domain.StockMovement) int { return a.Number.Seq - b.Number.Seq })
	return out, nil
}

func (r docRepo) SavePayment(d domain.Payment) error {
	r.store.payments[docKey(r.tenantID, d.Number)] = d
	return nil
}

func (r docRepo) GetPayment(n domain.DocNumber) (domain.Payment, error) {
	v, ok := r.store.payments[docKey(r.tenantID, n)]
	if !ok {
		return domain.Payment{}, app.ErrNotFound
	}
	return v, nil
}

func (r docRepo) LiveRectifyingNotes(number domain.DocNumber) ([]domain.SalesInvoice, error) {
	prefix := r.tenantID + sep
	target := number.Format()
	var out []domain.SalesInvoice
	for k, v := range r.store.sales {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if v.DocumentType != domain.NC && v.DocumentType != domain.ND {
			continue
		}
		if v.Status == domain.StatusCancelled {
			continue
		}
		if rectifies(v, target) {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b domain.SalesInvoice) int { return a.Number.Seq - b.Number.Seq })
	return out, nil
}

func rectifies(nc domain.SalesInvoice, target string) bool {
	for _, line := range nc.Lines {
		for _, ref := range line.References {
			if ref.Reference == target {
				return true
			}
			if pn, err := domain.ParseDocNumber(ref.Reference); err == nil && pn.Format() == target {
				return true
			}
		}
	}
	return false
}

func (r docRepo) SourceState(n domain.DocNumber, axis app.AllocAxis) (domain.SourceDocState, error) {
	src, ok := r.store.sales[docKey(r.tenantID, n)]
	if !ok {
		return domain.SourceDocState{}, app.ErrNotFound
	}
	return domain.SourceDocState{
		CustomerID: src.Customer.CustomerID,
		Status:     src.Status,
		Gross:      src.Totals.GrossTotal,
		Consumed:   r.consumedAgainst(n, axis),
	}, nil
}

// consumedAgainst sums non-cancelled allocations on the given axis:
//   - AllocSettlement: RC/RG receipt settlements (PaymentLine.SourceDocuments) only.
//   - AllocCredit: NC line grosses (References) only — ND is excluded.
func (r docRepo) consumedAgainst(n domain.DocNumber, axis app.AllocAxis) domain.Money {
	key := n.Format()
	prefix := r.tenantID + sep
	var sum domain.Money
	switch axis {
	case app.AllocCredit:
		for k, inv := range r.store.sales {
			if !strings.HasPrefix(k, prefix) || inv.Status == domain.StatusCancelled {
				continue
			}
			if inv.DocumentType != domain.NC { // ND excluded from credit axis
				continue
			}
			for _, ln := range inv.Lines {
				for _, ref := range ln.References {
					if ref.Reference == key {
						sum += ln.LineTotal()
					}
				}
			}
		}
	case app.AllocSettlement:
		for k, p := range r.store.payments {
			if !strings.HasPrefix(k, prefix) || p.Status == domain.StatusCancelled {
				continue
			}
			for _, ln := range p.Lines {
				amt := ln.Movement.Amount()
				if ln.SettlementAmount != nil {
					amt = *ln.SettlementAmount
				}
				for _, sd := range ln.SourceDocuments {
					if sd.OriginatingON == key {
						sum += amt
					}
				}
			}
		}
	}
	return sum
}

func (r docRepo) PaymentsInPeriod(from, to time.Time) ([]domain.Payment, error) {
	prefix := r.tenantID + sep
	var out []domain.Payment
	for k, v := range r.store.payments {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if v.TransactionDate.Before(from) || v.TransactionDate.After(to) {
			continue
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b domain.Payment) int { return a.Number.Seq - b.Number.Seq })
	return out, nil
}

type outboxRepo struct {
	store    *Store
	tenantID string
}

func (r outboxRepo) Enqueue(t app.Task) error {
	r.store.nextTaskID++
	t.ID = fmt.Sprint(r.store.nextTaskID)
	t.TenantID = r.tenantID // stamp from the bound tenant; do not trust the caller
	t.Status = app.TaskPending
	// NextAttempt zero value = immediately due; Attempts starts at 0.
	r.store.outbox = append(r.store.outbox, t)
	return nil
}

// ── OutboxQueue (cross-tenant, operational; called outside Run) ──────────────

func (s *Store) ClaimDue(now time.Time, limit int) ([]app.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var claimed []app.Task
	for i := range s.outbox {
		t := &s.outbox[i]
		if t.Status == app.TaskPending && !t.NextAttempt.After(now) {
			t.Status = app.TaskClaimed
			t.Attempts++
			claimed = append(claimed, *t)
			if len(claimed) >= limit {
				break
			}
		}
	}
	return claimed, nil
}

func (s *Store) Complete(id, atResult string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCompleteOnce {
		s.failCompleteOnce = false
		return fmt.Errorf("simulated Complete failure")
	}
	t := s.findTask(id)
	if t == nil {
		return app.ErrNotFound
	}
	t.Status = app.TaskDone
	t.ATResult = atResult
	return nil
}

func (s *Store) Fail(id, errMsg string, retryAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.findTask(id)
	if t == nil {
		return app.ErrNotFound
	}
	t.LastError = errMsg
	if retryAt != nil {
		t.Status = app.TaskPending
		t.NextAttempt = *retryAt
	} else {
		t.Status = app.TaskFailed
	}
	return nil
}

func (s *Store) Find(tenantID string, number domain.DocNumber) (app.Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.outbox {
		if s.outbox[i].TenantID == tenantID && s.outbox[i].Number == number {
			return s.outbox[i], true, nil
		}
	}
	return app.Task{}, false, nil
}

func (s *Store) findTask(id string) *app.Task {
	for i := range s.outbox {
		if s.outbox[i].ID == id {
			return &s.outbox[i]
		}
	}
	return nil
}

// OutboxTasks returns a copy of all tasks (test/inspection helper).
func (s *Store) OutboxTasks() []app.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.outbox)
}

type idemRepo struct {
	store    *Store
	tenantID string
}

func (r idemRepo) Get(key string) (app.IdempotencyRecord, error) {
	v, ok := r.store.idem[idemStoreKey(r.tenantID, key)]
	if !ok {
		return app.IdempotencyRecord{}, app.ErrNotFound
	}
	return v, nil
}

func (r idemRepo) Put(rec app.IdempotencyRecord) error {
	r.store.idem[idemStoreKey(r.tenantID, rec.Key)] = rec
	return nil
}

// compile-time interface checks
var (
	_ app.UnitOfWork  = (*Store)(nil)
	_ app.OutboxQueue = (*Store)(nil)
	_ app.RepoSet     = (*repoSet)(nil)
)
