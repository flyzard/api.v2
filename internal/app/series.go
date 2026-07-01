package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/flyzard/invoicing.v2/internal/adapter/at"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// SeriesService manages a tenant's document series against the AT SeriesWS.
// AT-mutating operations never hold a transaction across the SOAP call.
type SeriesService struct {
	tenants TenantStore
	uow     UnitOfWork
	clients ATClientFactory
	clock   Clock
}

func newSeriesService(d Deps) *SeriesService {
	return &SeriesService{tenants: d.Tenants, uow: d.UoW, clients: d.ATClients, clock: d.Clock}
}

// CreateSeries creates a series locally (unregistered). recovery=true makes it a
// recovery series (AT tipoSerie "R").
func (s *SeriesService) CreateSeries(ctx context.Context, tenantID, id string, dt domain.DocumentType, recovery bool) (domain.Series, error) {
	var (
		ser domain.Series
		err error
	)
	if recovery {
		ser, err = domain.NewRecoverySeries(id, dt)
	} else {
		ser, err = domain.NewSeries(id, dt)
	}
	if err != nil {
		return domain.Series{}, newError(KindInvalid, fmt.Errorf("new series %q: %w", id, err))
	}
	if rerr := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		return tx.Series().Create(ser)
	}); rerr != nil {
		if errors.Is(rerr, ErrAlreadyExists) {
			return domain.Series{}, newError(KindConflict, fmt.Errorf("series %q: %w", id, rerr))
		}
		return domain.Series{}, newError(KindInternal, rerr)
	}
	return ser, nil
}

// prepare resolves the tenant's AT series client and loads the current series.
func (s *SeriesService) prepare(ctx context.Context, tenantID, id string, dt domain.DocumentType) (at.SeriesClient, domain.Series, error) {
	tenant, err := s.tenants.Resolve(ctx, tenantID)
	if err != nil {
		return nil, domain.Series{}, newError(KindNotFound, fmt.Errorf("resolve tenant %q: %w", tenantID, err))
	}
	var cur domain.Series
	if lerr := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		var gerr error
		cur, gerr = tx.Series().Get(id, dt)
		return gerr
	}); lerr != nil {
		return nil, domain.Series{}, newError(KindNotFound, fmt.Errorf("series %q: %w", id, lerr))
	}
	sc, _, _, cerr := s.clients.ForTenant(tenant)
	if cerr != nil {
		return nil, domain.Series{}, newError(KindInternal, fmt.Errorf("at client: %w", cerr))
	}
	return sc, cur, nil
}

// applySeries re-loads the series and applies a domain transition under the
// optimistic version check (the AT call already succeeded).
func (s *SeriesService) applySeries(ctx context.Context, tenantID, id string, dt domain.DocumentType, apply func(*domain.Series) error) (domain.Series, error) {
	var out domain.Series
	err := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		latest, gerr := tx.Series().Get(id, dt)
		if gerr != nil {
			return newError(KindNotFound, fmt.Errorf("series %q: %w", id, gerr))
		}
		prev := latest.Version
		if aerr := apply(&latest); aerr != nil {
			return newError(KindConflict, fmt.Errorf("apply transition to %q: %w", id, aerr))
		}
		if serr := tx.Series().Save(latest, prev); serr != nil {
			if errors.Is(serr, ErrVersionConflict) {
				return newError(KindConflict, fmt.Errorf("series %q changed concurrently: %w", id, serr))
			}
			return newError(KindInternal, fmt.Errorf("save series %q: %w", id, serr))
		}
		out = latest
		return nil
	})
	if err != nil {
		return domain.Series{}, err
	}
	return out, nil
}

// RegisterSeries registers the series with AT (registarSerie) and adopts the
// returned validation code locally.
func (s *SeriesService) RegisterSeries(ctx context.Context, tenantID, id string, dt domain.DocumentType) (domain.Series, error) {
	sc, cur, err := s.prepare(ctx, tenantID, id, dt)
	if err != nil {
		return domain.Series{}, err
	}
	req, berr := at.RegistrationFor(cur, s.clock.Now())
	if berr != nil {
		return domain.Series{}, newError(KindConflict, fmt.Errorf("register %q: %w", id, berr))
	}
	res, aerr := sc.RegisterSeries(ctx, req)
	if aerr != nil {
		return domain.Series{}, newError(KindAT, fmt.Errorf("AT registarSerie %q: %w", id, aerr))
	}
	return s.applySeries(ctx, tenantID, id, dt, func(x *domain.Series) error {
		return x.RegisterWithAT(res.ValidationCode, res.RegistrationDate)
	})
}

// FinalizeSeries finalizes a registered series that has issued at least one
// document (finalizarSerie).
func (s *SeriesService) FinalizeSeries(ctx context.Context, tenantID, id string, dt domain.DocumentType, justification string) (domain.Series, error) {
	sc, cur, err := s.prepare(ctx, tenantID, id, dt)
	if err != nil {
		return domain.Series{}, err
	}
	req, berr := at.FinalizationFor(cur, justification)
	if berr != nil {
		return domain.Series{}, newError(KindConflict, fmt.Errorf("finalize %q: %w", id, berr))
	}
	if aerr := sc.FinalizeSeries(ctx, req); aerr != nil {
		return domain.Series{}, newError(KindAT, fmt.Errorf("AT finalizarSerie %q: %w", id, aerr))
	}
	return s.applySeries(ctx, tenantID, id, dt, func(x *domain.Series) error {
		return x.Finalize(s.clock.Now())
	})
}

// CancelSeries cancels a registered series that never issued a document
// (anularSerie).
func (s *SeriesService) CancelSeries(ctx context.Context, tenantID, id string, dt domain.DocumentType) (domain.Series, error) {
	sc, cur, err := s.prepare(ctx, tenantID, id, dt)
	if err != nil {
		return domain.Series{}, err
	}
	req, berr := at.CancellationFor(cur)
	if berr != nil {
		return domain.Series{}, newError(KindConflict, fmt.Errorf("cancel %q: %w", id, berr))
	}
	if aerr := sc.CancelSeries(ctx, req); aerr != nil {
		return domain.Series{}, newError(KindAT, fmt.Errorf("AT anularSerie %q: %w", id, aerr))
	}
	return s.applySeries(ctx, tenantID, id, dt, func(x *domain.Series) error {
		return x.Cancel(s.clock.Now())
	})
}

// SeedRegisteredSeries creates a series that is already registered with AT
// (e.g. migrated from another system). It builds the series, applies the AT
// registration locally, and persists it in one shot — no SOAP call is made.
func (s *SeriesService) SeedRegisteredSeries(ctx context.Context, tenantID, id, docType, atCode, registeredAt string) error {
	dt, e := mapDocType(docType)
	if e != nil {
		return e
	}
	ts, e := lisbonDate(registeredAt)
	if e != nil {
		return e
	}
	ser, err := domain.NewSeries(id, dt)
	if err != nil {
		return newError(KindInvalid, fmt.Errorf("new series %q: %w", id, err))
	}
	if rerr := ser.RegisterWithAT(atCode, ts); rerr != nil {
		return newError(KindInvalid, fmt.Errorf("register %q: %w", id, rerr))
	}
	if cerr := s.uow.Run(ctx, tenantID, func(tx RepoSet) error { return tx.Series().Create(ser) }); cerr != nil {
		if errors.Is(cerr, ErrAlreadyExists) {
			return newError(KindConflict, fmt.Errorf("series %q: %w", id, cerr))
		}
		return newError(KindInternal, cerr)
	}
	return nil
}

// ConsultSeries reads AT's view of the series (consultarSeries). Read-only.
func (s *SeriesService) ConsultSeries(ctx context.Context, tenantID, id string, dt domain.DocumentType) (at.SeriesStatus, error) {
	tenant, err := s.tenants.Resolve(ctx, tenantID)
	if err != nil {
		return at.SeriesStatus{}, newError(KindNotFound, fmt.Errorf("resolve tenant %q: %w", tenantID, err))
	}
	sc, _, _, cerr := s.clients.ForTenant(tenant)
	if cerr != nil {
		return at.SeriesStatus{}, newError(KindInternal, fmt.Errorf("at client: %w", cerr))
	}
	st, aerr := sc.GetSeriesStatus(ctx, id, dt)
	if aerr != nil {
		return at.SeriesStatus{}, newError(KindAT, fmt.Errorf("AT consultarSeries %q: %w", id, aerr))
	}
	return *st, nil
}
