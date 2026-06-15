package app

import (
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Deps is the wiring surface for the application layer.
type Deps struct {
	Tenants   TenantStore
	UoW       UnitOfWork
	Queue     OutboxQueue
	Clock     Clock
	Signer    domain.Signer
	ATClients ATClientFactory
	Software  config.SoftwareIdentity
}

// Services is the set of application services exposed to transports.
type Services struct {
	Invoicing *InvoicingService
	Comm      *CommService
	Series    *SeriesService
	Export    *ExportService
	Query     *QueryService
}

func New(d Deps) *Services {
	return &Services{
		Invoicing: newInvoicingService(d),
		Comm:      newCommService(d),
		Series:    newSeriesService(d),
		Export:    newExportService(d),
		Query:     newQueryService(d),
	}
}
