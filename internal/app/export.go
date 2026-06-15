package app

import (
	"context"
	"fmt"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/saft"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// ExportService projects a tenant's documents to SAF-T (PT) XML for a period.
type ExportService struct {
	tenants  TenantStore
	uow      UnitOfWork
	clock    Clock
	software config.SoftwareIdentity
}

func newExportService(d Deps) *ExportService {
	return &ExportService{tenants: d.Tenants, uow: d.UoW, clock: d.Clock, software: d.Software}
}

// ExportSAFT builds a SAF-T (PT) file for [from, to], projecting every document
// family the tenant issued in the period (sales, stock movements, work documents,
// payments).
func (s *ExportService) ExportSAFT(ctx context.Context, tenantID string, from, to time.Time) ([]byte, error) {
	tenant, err := s.tenants.Resolve(ctx, tenantID)
	if err != nil {
		return nil, newError(KindNotFound, fmt.Errorf("resolve tenant %q: %w", tenantID, err))
	}
	var (
		sales    []domain.SalesInvoice
		stock    []domain.StockMovement
		work     []domain.WorkDocument
		payments []domain.Payment
	)
	if lerr := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		var gerr error
		if sales, gerr = tx.Documents().SalesInPeriod(from, to); gerr != nil {
			return gerr
		}
		if stock, gerr = tx.Documents().StockInPeriod(from, to); gerr != nil {
			return gerr
		}
		if work, gerr = tx.Documents().WorkInPeriod(from, to); gerr != nil {
			return gerr
		}
		payments, gerr = tx.Documents().PaymentsInPeriod(from, to)
		return gerr
	}); lerr != nil {
		return nil, newError(KindInternal, fmt.Errorf("load documents: %w", lerr))
	}

	hdr := saft.Header{
		Issuer: tenant.Company,
		Software: saft.SoftwareIdentity{
			ProducerTaxID:     s.software.ProducerTaxID,
			CertificateNumber: s.software.CertificateNumber,
			ProductID:         s.software.ProductID(),
			Version:           s.software.Version,
		},
		Start:     from,
		End:       to,
		CreatedAt: s.clock.Now(),
	}
	out, err := saft.Export(hdr, sales, stock, work, payments)
	if err != nil {
		return nil, newError(KindInternal, fmt.Errorf("saft export: %w", err))
	}
	return out, nil
}
