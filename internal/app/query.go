package app

import (
	"context"
	"fmt"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/pdf"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// DocumentView is a sales invoice joined with its AT-communication status.
type DocumentView struct {
	Invoice domain.SalesInvoice
	Comm    *Task // nil if the document was never queued for communication
}

// QueryService is the read side: fetch, list, comm status, and PDF rendering.
type QueryService struct {
	tenants  TenantStore
	uow      UnitOfWork
	queue    OutboxQueue
	software config.SoftwareIdentity
}

func newQueryService(d Deps) *QueryService {
	return &QueryService{tenants: d.Tenants, uow: d.UoW, queue: d.Queue, software: d.Software}
}

func (s *QueryService) loadInvoice(ctx context.Context, tenantID string, number domain.DocNumber) (domain.SalesInvoice, error) {
	var inv domain.SalesInvoice
	if err := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		var gerr error
		inv, gerr = tx.Documents().GetSalesInvoice(number)
		return gerr
	}); err != nil {
		return domain.SalesInvoice{}, newError(KindNotFound, fmt.Errorf("document %s: %w", number.Format(), err))
	}
	return inv, nil
}

// GetDocument returns the invoice joined with its communication status.
func (s *QueryService) GetDocument(ctx context.Context, tenantID string, number domain.DocNumber) (DocumentView, error) {
	inv, err := s.loadInvoice(ctx, tenantID, number)
	if err != nil {
		return DocumentView{}, err
	}
	view := DocumentView{Invoice: inv}
	task, ok, err := s.queue.Find(tenantID, number)
	if err != nil {
		return DocumentView{}, newError(KindInternal, fmt.Errorf("comm status %s: %w", number.Format(), err))
	}
	if ok {
		view.Comm = &task
	}
	return view, nil
}

// ListSales lists the tenant's sales invoices issued within [from, to].
func (s *QueryService) ListSales(ctx context.Context, tenantID string, from, to time.Time) ([]domain.SalesInvoice, error) {
	var out []domain.SalesInvoice
	if err := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		var gerr error
		out, gerr = tx.Documents().SalesInPeriod(from, to)
		return gerr
	}); err != nil {
		return nil, newError(KindInternal, fmt.Errorf("list sales: %w", err))
	}
	return out, nil
}

// CommStatus returns the communication task for a document.
func (s *QueryService) CommStatus(ctx context.Context, tenantID string, number domain.DocNumber) (Task, error) {
	task, ok, err := s.queue.Find(tenantID, number)
	if err != nil {
		return Task{}, newError(KindInternal, err)
	}
	if !ok {
		return Task{}, newError(KindNotFound, fmt.Errorf("no communication task for %s", number.Format()))
	}
	return task, nil
}

// RenderPDF renders the original copy of a sales invoice as a PDF.
func (s *QueryService) RenderPDF(ctx context.Context, tenantID string, number domain.DocNumber) ([]byte, error) {
	tenant, err := s.tenants.Resolve(ctx, tenantID)
	if err != nil {
		return nil, newError(KindNotFound, fmt.Errorf("resolve tenant %q: %w", tenantID, err))
	}
	inv, err := s.loadInvoice(ctx, tenantID, number)
	if err != nil {
		return nil, err
	}
	meta := pdf.Meta{
		Seller: pdf.Seller{
			Name:       tenant.Company.Name,
			TaxID:      string(tenant.Company.NIF),
			Address:    tenant.Company.Address.AddressDetail,
			City:       tenant.Company.Address.City,
			PostalCode: tenant.Company.Address.PostalCode,
		},
		CertNumber: s.software.CertificateNumber,
		Copy:       pdf.Original,
	}
	out, rerr := pdf.RenderSalesInvoice(inv, meta)
	if rerr != nil {
		return nil, newError(KindInternal, fmt.Errorf("render pdf %s: %w", number.Format(), rerr))
	}
	return out, nil
}
