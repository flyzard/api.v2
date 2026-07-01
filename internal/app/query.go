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

// CopyKind is the app-owned copy designation for PDF rendering.
// Consumers never need to import internal/adapter/pdf.
type CopyKind int

const (
	Original CopyKind = iota
	Duplicado
	Triplicado
	Quadruplicado
	SegundaVia
)

var copyToPDF = map[CopyKind]pdf.CopyKind{
	Original:      pdf.Original,
	Duplicado:     pdf.Duplicado,
	Triplicado:    pdf.Triplicado,
	Quadruplicado: pdf.Quadruplicado,
	SegundaVia:    pdf.SegundaVia,
}

var pdfToApp = func() map[pdf.CopyKind]CopyKind {
	m := make(map[pdf.CopyKind]CopyKind, len(copyToPDF))
	for ak, pk := range copyToPDF {
		m[pk] = ak
	}
	return m
}()

// RequiredVias returns the legally required print copies for the given document type string.
// Returns a KindInvalid error for unknown types.
func RequiredVias(docType string) ([]CopyKind, error) {
	dt, aerr := mapDocType(docType)
	if aerr != nil {
		return nil, aerr
	}
	pdfVias := pdf.RequiredVias(dt)
	out := make([]CopyKind, len(pdfVias))
	for i, pv := range pdfVias {
		out[i] = pdfToApp[pv]
	}
	return out, nil
}

// RenderPDF renders the requested copy of a document as PDF bytes.
// number is the formatted document number (e.g. "FT2026/1").
func (s *QueryService) RenderPDF(ctx context.Context, tenantID, number string, copy CopyKind) ([]byte, error) {
	tenant, err := s.tenants.Resolve(ctx, tenantID)
	if err != nil {
		return nil, newError(KindNotFound, fmt.Errorf("resolve tenant %q: %w", tenantID, err))
	}
	n, nerr := parseNumber(number)
	if nerr != nil {
		return nil, nerr
	}
	pdfCopy, ok := copyToPDF[copy]
	if !ok {
		return nil, newErrorCode(KindInvalid, "unknown_copy_kind", fmt.Errorf("unknown copy kind %d", copy))
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
		Copy:       pdfCopy,
	}
	var out []byte
	var rerr error
	txErr := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		switch {
		case n.Type.IsSales():
			inv, gerr := tx.Documents().GetSalesInvoice(n)
			if gerr != nil {
				return gerr
			}
			out, rerr = pdf.RenderSalesInvoice(inv, meta)
		case n.Type.IsWorking():
			wd, gerr := tx.Documents().GetWorkDocument(n)
			if gerr != nil {
				return gerr
			}
			out, rerr = pdf.RenderWorkDocument(wd, meta)
		case n.Type.IsTransport():
			sm, gerr := tx.Documents().GetStockMovement(n)
			if gerr != nil {
				return gerr
			}
			out, rerr = pdf.RenderStockMovement(sm, meta)
		case n.Type.IsReceipt():
			p, gerr := tx.Documents().GetPayment(n)
			if gerr != nil {
				return gerr
			}
			out, rerr = pdf.RenderPayment(p, meta)
		default:
			return newError(KindInvalid, fmt.Errorf("unsupported document type %q for PDF rendering", n.Type))
		}
		return nil
	})
	if txErr != nil {
		return nil, newError(KindNotFound, fmt.Errorf("document %s: %w", number, txErr))
	}
	if rerr != nil {
		return nil, newError(KindInternal, fmt.Errorf("render pdf %s: %w", number, rerr))
	}
	return out, nil
}
