package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// parseableSourceNums returns the formatted doc numbers for every parseable
// DocReference in NC/ND lines. Used to collect source-lock keys before issuance.
func parseableSourceNums(lines []domain.DocumentLine) []string {
	var out []string
	for _, line := range lines {
		for _, ref := range line.References {
			if n, err := domain.ParseDocNumber(ref.Reference); err == nil {
				out = append(out, n.Format())
			}
		}
	}
	return out
}

// parseablePaymentSourceNums returns the formatted doc numbers for every
// parseable OriginatingON in RC/RG payment lines. Used to collect source-lock
// keys before issuance.
func parseablePaymentSourceNums(lines []domain.PaymentLine) []string {
	var out []string
	for _, line := range lines {
		for _, src := range line.SourceDocuments {
			if n, err := domain.ParseDocNumber(src.OriginatingON); err == nil {
				out = append(out, n.Format())
			}
		}
	}
	return out
}

// allocationClaimsSales builds the claims map (source doc number → gross claim)
// for NC/ND lines from their DocReferences. Lines with unparseable references
// are silently skipped (AllowUnknownSource will permit them).
func allocationClaimsSales(lines []domain.DocumentLine) map[string]domain.Money {
	claims := make(map[string]domain.Money)
	for _, line := range lines {
		for _, ref := range line.References {
			if _, err := domain.ParseDocNumber(ref.Reference); err != nil {
				continue // unparseable → skip; AllowUnknownSource covers it
			}
			claims[ref.Reference] += line.LineTotal()
		}
	}
	return claims
}

// validateSalesAllocations fetches SourceDocState for each claim and calls
// domain.ValidateAllocations. ErrNotFound sources are silently dropped so that
// AllowUnknownSource can permit them. Any validation error maps to KindInvalid.
// axis selects which prior allocations count toward the ceiling (AllocCredit for
// NC/ND; the axis is always AllocCredit because ND uses SkipSourceCeiling instead).
func (s *InvoicingService) validateSalesAllocations(
	tx RepoSet,
	draft *domain.DraftSalesInvoice,
	claims map[string]domain.Money,
	policy domain.AllocationPolicy,
	axis AllocAxis,
) error {
	sources := make(map[string]domain.SourceDocState, len(claims))
	for ref := range claims {
		n, err := domain.ParseDocNumber(ref)
		if err != nil {
			continue
		}
		st, err := tx.Documents().SourceState(n, axis)
		if errors.Is(err, ErrNotFound) {
			continue // AllowUnknownSource will handle the missing entry
		}
		if err != nil {
			return newError(KindInternal, fmt.Errorf("source state for %s: %w", ref, err))
		}
		sources[ref] = st
	}
	if err := domain.ValidateAllocations(draft.Customer.CustomerID, claims, sources, policy); err != nil {
		return newError(KindInvalid, err)
	}
	return nil
}

const maxIssueAttempts = 3

// IdempotencyKey deduplicates issuance across client retries. Key is the
// client-supplied token; Fingerprint is a hash of the request payload, so a
// reused key with a different payload is rejected rather than replaying the
// wrong document.
type IdempotencyKey struct {
	Key         string
	Fingerprint string
}

// Fingerprint is the canonical idempotency fingerprint: hex(sha256(payload)).
// A transport computes it over the raw request body and sets it on
// IdempotencyKey.Fingerprint, so a reused Key with a changed payload is
// rejected (ErrIdempotencyMismatch) instead of replaying the wrong document.
func Fingerprint(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// IssueSalesInvoiceRequest bundles the issuance args for a sales-family document.
// Draft stays a domain type — the boundary is thin (no DTO layer).
type IssueSalesInvoiceRequest struct {
	Draft    domain.DraftSalesInvoice
	SeriesID string
	SourceID string
	Idem     IdempotencyKey
}

type IssueWorkDocumentRequest struct {
	Draft    domain.DraftWorkDocument
	SeriesID string
	SourceID string
	Idem     IdempotencyKey
}

type IssueStockMovementRequest struct {
	Draft    domain.DraftStockMovement
	SeriesID string
	SourceID string
	Idem     IdempotencyKey
}

// IssuePaymentRequest is deliberately asymmetric: it carries Totals
// (domain.IssuePayment takes caller-supplied totals, not recomputed from lines)
// and has no SourceID (receipts carry no Hash/HashControl). Do not "normalise" it.
type IssuePaymentRequest struct {
	Draft    domain.PaymentDraft
	SeriesID string
	Totals   domain.PaymentTotals
	Idem     IdempotencyKey
}

// IntegrateRecoveredSalesInvoiceRequest mirrors IssueSalesInvoiceRequest with an
// additional RecoveredRef that identifies the original document (Portaria 363/2010).
type IntegrateRecoveredSalesInvoiceRequest struct {
	Draft        domain.DraftSalesInvoice
	SeriesID     string
	SourceID     string
	RecoveredRef domain.RecoveredRef
	Idem         IdempotencyKey
}

type IntegrateRecoveredWorkDocumentRequest struct {
	Draft        domain.DraftWorkDocument
	SeriesID     string
	SourceID     string
	RecoveredRef domain.RecoveredRef
	Idem         IdempotencyKey
}

type IntegrateRecoveredStockMovementRequest struct {
	Draft        domain.DraftStockMovement
	SeriesID     string
	SourceID     string
	RecoveredRef domain.RecoveredRef
	Idem         IdempotencyKey
}

// IntegrateRecoveredPaymentRequest omits SourceID/RecoveredRef because receipts
// carry no Hash/HashControl in SAF-T; recovery materialises as SourcePayment="M".
type IntegrateRecoveredPaymentRequest struct {
	Draft    domain.PaymentDraft
	SeriesID string
	Totals   domain.PaymentTotals
	Idem     IdempotencyKey
}

// InvoicingService issues documents through the domain, persisting the document
// and the advanced series in a single transaction.
type InvoicingService struct {
	tenants     TenantStore
	uow         UnitOfWork
	clock       Clock
	signer      domain.Signer
	software    config.SoftwareIdentity
	locks       *seriesLocks
	sourceLocks *sourceLocks
}

func newInvoicingService(d Deps) *InvoicingService {
	return &InvoicingService{
		tenants:     d.Tenants,
		uow:         d.UoW,
		clock:       d.Clock,
		signer:      d.Signer,
		software:    d.Software,
		locks:       newSeriesLocks(),
		sourceLocks: newSourceLocks(),
	}
}

// IssueSalesInvoice issues one sales-family document (FT/FS/FR/NC/ND).
func (s *InvoicingService) IssueSalesInvoice(
	ctx context.Context, tenantID string, req IssueSalesInvoiceRequest,
) (domain.SalesInvoice, error) {
	draft, seriesID, sourceID, idem := req.Draft, req.SeriesID, req.SourceID, req.Idem
	// For allocation-bearing documents (NC/ND), acquire per-source locks before
	// entering the transaction so that validation + issue + save are atomic per
	// source. Locks are acquired in sorted order to prevent deadlock when a single
	// issuance references multiple sources.
	if dt := draft.DocumentType; dt == domain.NC || dt == domain.ND {
		sources := parseableSourceNums(draft.Lines)
		unlock := s.sourceLocks.lockMany(tenantID, sources)
		defer unlock()
	}
	return chainIssue(s, ctx, tenantID, seriesID, draft.DocumentType, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.SalesInvoice, error) {
			return tx.Documents().GetSalesInvoice(n)
		},
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.SalesInvoice, error) {
			draft.Series = *series // authoritative series from the repo (domain validation requires it)
			opts := issueOptions(tenant)
			dt := draft.DocumentType
			if dt == domain.ND {
				opts.Reader = docReader{tx.Documents()}
			}
			if dt == domain.NC || dt == domain.ND {
				claims := allocationClaimsSales(draft.Lines)
				policy := domain.AllocationPolicy{
					AllowUnknownSource: tenant.AllowUnknownAllocSource,
					// ND has no ceiling against the source gross (debit notes raise
					// the invoice amount; they never "consume" it). Status + customer
					// checks still run via ValidateAllocations.
					// NC ceiling is relaxable via AllowRappelNC (rappel discount NCs).
					SkipSourceCeiling: dt == domain.ND || (dt == domain.NC && tenant.AllowRappelNC),
				}
				if verr := s.validateSalesAllocations(tx, &draft, claims, policy, AllocCredit); verr != nil {
					return domain.SalesInvoice{}, verr
				}
			}
			return domain.IssueSalesInvoice(&draft, series, s.signer, sourceID, s.clock.Now(), opts, qrFor(tenant, s.software))
		},
		func(d domain.SalesInvoice) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.SalesInvoice) error { return tx.Documents().SaveSalesInvoice(d) },
		func(tx RepoSet, tenant Tenant, d domain.SalesInvoice) error {
			// fatcorews invoice communication is a per-tenant DL 28/2019 election.
			if tenant.CommMode != CommRealtime {
				return nil
			}
			if eerr := tx.Outbox().Enqueue(Task{TenantID: tenantID, Kind: KindInvoiceComm, Number: d.Number}); eerr != nil {
				return newError(KindInternal, fmt.Errorf("enqueue comm: %w", eerr))
			}
			return nil
		},
	)
}

// IssueWorkDocument issues one work document (NE/OR/PF/CM/FC/FO/OU). Like a sales
// invoice it is signed and advances the per-series hash chain. Work documents are
// not communicated to AT, so issuance enqueues nothing.
func (s *InvoicingService) IssueWorkDocument(
	ctx context.Context, tenantID string, req IssueWorkDocumentRequest,
) (domain.WorkDocument, error) {
	draft, seriesID, sourceID, idem := req.Draft, req.SeriesID, req.SourceID, req.Idem
	return chainIssue(s, ctx, tenantID, seriesID, draft.DocumentType, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.WorkDocument, error) {
			return tx.Documents().GetWorkDocument(n)
		},
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.WorkDocument, error) {
			draft.Series = *series
			return domain.IssueWorkDocument(&draft, series, s.signer, sourceID, s.clock.Now(), issueOptions(tenant), qrFor(tenant, s.software))
		},
		func(d domain.WorkDocument) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.WorkDocument) error { return tx.Documents().SaveWorkDocument(d) },
		nil,
	)
}

// IssueStockMovement issues one stock-movement / transport document
// (GR/GT/GA/GC/GD). Signed and hash-chained like a sales invoice. When the
// tenant elects transport communication (CommTransport), issuance enqueues a
// KindTransportComm outbox task (DL 147/2003 sgdtws) in the same transaction.
func (s *InvoicingService) IssueStockMovement(
	ctx context.Context, tenantID string, req IssueStockMovementRequest,
) (domain.StockMovement, error) {
	draft, seriesID, sourceID, idem := req.Draft, req.SeriesID, req.SourceID, req.Idem
	return chainIssue(s, ctx, tenantID, seriesID, draft.DocumentType, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.StockMovement, error) {
			return tx.Documents().GetStockMovement(n)
		},
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.StockMovement, error) {
			draft.Series = *series
			return domain.IssueStockMovement(&draft, series, s.signer, sourceID, s.clock.Now(), issueOptions(tenant), qrFor(tenant, s.software))
		},
		func(d domain.StockMovement) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.StockMovement) error { return tx.Documents().SaveStockMovement(d) },
		func(tx RepoSet, tenant Tenant, d domain.StockMovement) error {
			if !tenant.CommTransport {
				return nil
			}
			if eerr := tx.Outbox().Enqueue(Task{TenantID: tenantID, Kind: KindTransportComm, Number: d.Number}); eerr != nil {
				return newError(KindInternal, fmt.Errorf("enqueue transport comm: %w", eerr))
			}
			return nil
		},
	)
}

// IssuePayment issues one receipt (RC/RG). Unlike the other families a payment
// carries no signature or hash chain (no Hash/HashControl per the SAF-T XSD) and
// its totals are caller-supplied rather than recomputed from lines; it still
// advances the series sequence under the optimistic-version guard, so it shares
// the same spine.
func (s *InvoicingService) IssuePayment(
	ctx context.Context, tenantID string, req IssuePaymentRequest,
) (domain.Payment, error) {
	draft, seriesID, totals, idem := req.Draft, req.SeriesID, req.Totals, req.Idem
	// For receipts (RC/RG), acquire per-source locks before entering the
	// transaction. The ceiling is HARD for receipts and the race is most acute
	// here: two goroutines in different series can simultaneously pass the
	// ceiling check and together exceed the source gross.
	{
		sources := parseablePaymentSourceNums(draft.Lines)
		unlock := s.sourceLocks.lockMany(tenantID, sources)
		defer unlock()
	}
	return chainIssue(s, ctx, tenantID, seriesID, draft.Type, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.Payment, error) { return tx.Documents().GetPayment(n) },
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.Payment, error) {
			// Build allocation claims from payment lines before issuing — a rejection
			// must never advance the series counter.
			claims := allocationClaimsPayment(draft.Lines)
			policy := domain.AllocationPolicy{
				AllowUnknownSource: true, // pre-system invoices are common; receipt ceiling is HARD
			}
			if verr := s.validatePaymentAllocations(tx, &draft, claims, policy); verr != nil {
				return domain.Payment{}, verr
			}
			return domain.IssuePayment(&draft, series, s.clock.Now(), totals, issueOptions(tenant), qrFor(tenant, s.software))
		},
		func(d domain.Payment) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.Payment) error { return tx.Documents().SavePayment(d) },
		nil,
	)
}

// IntegrateRecoveredSalesInvoice is the recovery twin of IssueSalesInvoice.
// It routes through the same chainIssue spine (per-series lock, idempotent replay,
// optimistic retry, transactional save+outbox+idempotency) changing only the domain
// call to domain.IntegrateRecoveredSalesInvoice which forces SourceBilling="M" and
// encodes ref into the HashControl. The series must be a recovery series; the domain
// enforces this (ErrNotRecoverySeries / ErrRecoverySeriesMisuse → KindInvalid).
func (s *InvoicingService) IntegrateRecoveredSalesInvoice(
	ctx context.Context, tenantID string, req IntegrateRecoveredSalesInvoiceRequest,
) (domain.SalesInvoice, error) {
	draft, seriesID, sourceID, ref, idem := req.Draft, req.SeriesID, req.SourceID, req.RecoveredRef, req.Idem
	if dt := draft.DocumentType; dt == domain.NC || dt == domain.ND {
		sources := parseableSourceNums(draft.Lines)
		unlock := s.sourceLocks.lockMany(tenantID, sources)
		defer unlock()
	}
	return chainIssue(s, ctx, tenantID, seriesID, draft.DocumentType, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.SalesInvoice, error) {
			return tx.Documents().GetSalesInvoice(n)
		},
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.SalesInvoice, error) {
			draft.Series = *series
			opts := issueOptions(tenant)
			dt := draft.DocumentType
			if dt == domain.ND {
				opts.Reader = docReader{tx.Documents()}
			}
			if dt == domain.NC || dt == domain.ND {
				claims := allocationClaimsSales(draft.Lines)
				policy := domain.AllocationPolicy{
					AllowUnknownSource: tenant.AllowUnknownAllocSource,
					SkipSourceCeiling:  dt == domain.ND || (dt == domain.NC && tenant.AllowRappelNC),
				}
				if verr := s.validateSalesAllocations(tx, &draft, claims, policy, AllocCredit); verr != nil {
					return domain.SalesInvoice{}, verr
				}
			}
			return domain.IntegrateRecoveredSalesInvoice(&draft, ref, series, s.signer, sourceID, s.clock.Now(), opts, qrFor(tenant, s.software))
		},
		func(d domain.SalesInvoice) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.SalesInvoice) error { return tx.Documents().SaveSalesInvoice(d) },
		func(tx RepoSet, tenant Tenant, d domain.SalesInvoice) error {
			if tenant.CommMode != CommRealtime {
				return nil
			}
			if eerr := tx.Outbox().Enqueue(Task{TenantID: tenantID, Kind: KindInvoiceComm, Number: d.Number}); eerr != nil {
				return newError(KindInternal, fmt.Errorf("enqueue comm: %w", eerr))
			}
			return nil
		},
	)
}

// IntegrateRecoveredWorkDocument is the recovery twin of IssueWorkDocument.
func (s *InvoicingService) IntegrateRecoveredWorkDocument(
	ctx context.Context, tenantID string, req IntegrateRecoveredWorkDocumentRequest,
) (domain.WorkDocument, error) {
	draft, seriesID, sourceID, ref, idem := req.Draft, req.SeriesID, req.SourceID, req.RecoveredRef, req.Idem
	return chainIssue(s, ctx, tenantID, seriesID, draft.DocumentType, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.WorkDocument, error) {
			return tx.Documents().GetWorkDocument(n)
		},
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.WorkDocument, error) {
			draft.Series = *series
			return domain.IntegrateRecoveredWorkDocument(&draft, ref, series, s.signer, sourceID, s.clock.Now(), issueOptions(tenant), qrFor(tenant, s.software))
		},
		func(d domain.WorkDocument) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.WorkDocument) error { return tx.Documents().SaveWorkDocument(d) },
		nil,
	)
}

// IntegrateRecoveredStockMovement is the recovery twin of IssueStockMovement.
func (s *InvoicingService) IntegrateRecoveredStockMovement(
	ctx context.Context, tenantID string, req IntegrateRecoveredStockMovementRequest,
) (domain.StockMovement, error) {
	draft, seriesID, sourceID, ref, idem := req.Draft, req.SeriesID, req.SourceID, req.RecoveredRef, req.Idem
	return chainIssue(s, ctx, tenantID, seriesID, draft.DocumentType, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.StockMovement, error) {
			return tx.Documents().GetStockMovement(n)
		},
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.StockMovement, error) {
			draft.Series = *series
			return domain.IntegrateRecoveredStockMovement(&draft, ref, series, s.signer, sourceID, s.clock.Now(), issueOptions(tenant), qrFor(tenant, s.software))
		},
		func(d domain.StockMovement) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.StockMovement) error { return tx.Documents().SaveStockMovement(d) },
		nil,
	)
}

// IntegrateRecoveredPayment is the recovery twin of IssuePayment.
// Receipts carry no Hash/HashControl; recovery materialises as SourcePayment="M".
// There is no SourceID or RecoveredRef on the request — parallel to IssuePayment.
func (s *InvoicingService) IntegrateRecoveredPayment(
	ctx context.Context, tenantID string, req IntegrateRecoveredPaymentRequest,
) (domain.Payment, error) {
	draft, seriesID, totals, idem := req.Draft, req.SeriesID, req.Totals, req.Idem
	{
		sources := parseablePaymentSourceNums(draft.Lines)
		unlock := s.sourceLocks.lockMany(tenantID, sources)
		defer unlock()
	}
	return chainIssue(s, ctx, tenantID, seriesID, draft.Type, idem,
		func(tx RepoSet, n domain.DocNumber) (domain.Payment, error) { return tx.Documents().GetPayment(n) },
		func(tx RepoSet, tenant Tenant, series *domain.Series) (domain.Payment, error) {
			claims := allocationClaimsPayment(draft.Lines)
			policy := domain.AllocationPolicy{
				AllowUnknownSource: true, // pre-system invoices are common; receipt ceiling is HARD
			}
			if verr := s.validatePaymentAllocations(tx, &draft, claims, policy); verr != nil {
				return domain.Payment{}, verr
			}
			return domain.IntegrateRecoveredPayment(&draft, series, s.clock.Now(), totals, qrFor(tenant, s.software))
		},
		func(d domain.Payment) domain.DocNumber { return d.Number },
		func(tx RepoSet, d domain.Payment) error { return tx.Documents().SavePayment(d) },
		nil,
	)
}

// allocationClaimsPayment builds the claims map for RC/RG payment lines.
// Each SourceDocumentID contributes its SettlementAmount if present, otherwise
// the full line Movement.Amount() is claimed against each referenced source document.
func allocationClaimsPayment(lines []domain.PaymentLine) map[string]domain.Money {
	claims := make(map[string]domain.Money)
	for _, line := range lines {
		for _, src := range line.SourceDocuments {
			if _, err := domain.ParseDocNumber(src.OriginatingON); err != nil {
				continue // unparseable (e.g. "Adiantamento") → skip; AllowUnknownSource covers it
			}
			var amount domain.Money
			if line.SettlementAmount != nil {
				amount = *line.SettlementAmount
			} else {
				amount = line.Movement.Amount()
			}
			claims[src.OriginatingON] += amount
		}
	}
	return claims
}

// validatePaymentAllocations fetches SourceDocState for each claim on the
// AllocSettlement axis and calls domain.ValidateAllocations. ErrNotFound sources
// are silently dropped so that AllowUnknownSource can permit them. Any validation
// error maps to KindInvalid.
func (s *InvoicingService) validatePaymentAllocations(
	tx RepoSet,
	draft *domain.PaymentDraft,
	claims map[string]domain.Money,
	policy domain.AllocationPolicy,
) error {
	sources := make(map[string]domain.SourceDocState, len(claims))
	for ref := range claims {
		n, err := domain.ParseDocNumber(ref)
		if err != nil {
			continue
		}
		st, err := tx.Documents().SourceState(n, AllocSettlement)
		if errors.Is(err, ErrNotFound) {
			continue // AllowUnknownSource will handle the missing entry
		}
		if err != nil {
			return newError(KindInternal, fmt.Errorf("source state for %s: %w", ref, err))
		}
		sources[ref] = st
	}
	if err := domain.ValidateAllocations(draft.Customer.CustomerID, claims, sources, policy); err != nil {
		return newError(KindInvalid, err)
	}
	return nil
}

// chainIssue is the issuance spine shared by every family: per-series lock,
// idempotent replay, optimistic-version retry, and a single transaction that
// issues the document, persists it, advances the series, optionally runs a
// post-issue side effect (onIssued, e.g. comm enqueue), and records the
// idempotency key. The family-specific closures plug in the domain call and repo
// access; issue mutates the *Series it is handed and chainIssue saves that value.
func chainIssue[D any](
	s *InvoicingService, ctx context.Context,
	tenantID, seriesID string, dt domain.DocumentType, idem IdempotencyKey,
	load func(tx RepoSet, n domain.DocNumber) (D, error),
	issue func(tx RepoSet, tenant Tenant, series *domain.Series) (D, error),
	number func(D) domain.DocNumber,
	save func(tx RepoSet, doc D) error,
	onIssued func(tx RepoSet, tenant Tenant, doc D) error,
) (D, error) {
	var zero D
	tenant, err := s.tenants.Resolve(ctx, tenantID)
	if err != nil {
		return zero, newError(KindNotFound, fmt.Errorf("resolve tenant %q: %w", tenantID, err))
	}

	// Serialize issuance per series so the hash chain stays intact on the fast
	// (single-process) path; the repo version check is the cross-process backstop.
	unlock := s.locks.lock(tenantID, seriesID)
	defer unlock()

	var issued D
	var lastConflict error
	for range maxIssueAttempts {
		runErr := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
			// Idempotent replay.
			rec, gerr := tx.Idempotency().Get(idem.Key)
			switch {
			case gerr == nil:
				if rec.Fingerprint != idem.Fingerprint {
					return newError(KindConflict, fmt.Errorf("%w: %q", ErrIdempotencyMismatch, idem.Key))
				}
				prev, perr := load(tx, rec.DocNumber)
				if perr != nil {
					return newError(KindInternal, fmt.Errorf("replay load %s: %w", rec.DocNumber.Format(), perr))
				}
				issued = prev
				return nil
			case errors.Is(gerr, ErrNotFound):
				// not seen before — issue it
			default:
				return newError(KindInternal, fmt.Errorf("idempotency get: %w", gerr))
			}

			series, serr := tx.Series().Get(seriesID, dt)
			if serr != nil {
				return newError(KindNotFound, fmt.Errorf("series %q: %w", seriesID, serr))
			}
			if !series.CanIssue() {
				return newError(KindConflict, fmt.Errorf("%w: %q", ErrSeriesNotIssuable, seriesID))
			}
			prevVersion := series.Version

			doc, ierr := issue(tx, tenant, &series)
			if ierr != nil {
				return newError(KindInvalid, fmt.Errorf("issue %s: %w", dt, ierr))
			}

			if derr := save(tx, doc); derr != nil {
				return newError(KindInternal, fmt.Errorf("save document: %w", derr))
			}
			if verr := tx.Series().Save(series, prevVersion); verr != nil {
				return verr // ErrVersionConflict bubbles to the retry loop
			}
			if onIssued != nil {
				if eerr := onIssued(tx, tenant, doc); eerr != nil {
					return eerr
				}
			}
			if perr := tx.Idempotency().Put(IdempotencyRecord{Key: idem.Key, Fingerprint: idem.Fingerprint, DocNumber: number(doc)}); perr != nil {
				return newError(KindInternal, fmt.Errorf("idempotency put: %w", perr))
			}
			issued = doc
			return nil
		})
		if runErr == nil {
			return issued, nil
		}
		if errors.Is(runErr, ErrVersionConflict) {
			lastConflict = runErr
			continue
		}
		return zero, runErr
	}
	return zero, newError(KindConflict, fmt.Errorf("issuance exhausted %d attempts: %w", maxIssueAttempts, lastConflict))
}

// docReader adapts the document repo to domain.IssuedDocumentReader so issuance
// invariants that reference other documents (e.g. ND's product set) can resolve
// them. Only sales-family documents are referenced today.
type docReader struct{ docs DocumentRepo }

func (r docReader) FindByNumber(n domain.DocNumber) (domain.IssuedDocument, error) {
	inv, err := r.docs.GetSalesInvoice(n)
	if err != nil {
		return domain.IssuedDocument{}, err
	}
	return inv.IssuedDocument, nil
}

// CancelDocument cancels an issued sales invoice (Status N → A) when the
// e-Fatura cancellation deadline has not passed. It is the only sanctioned
// post-issuance mutation of an issued document and changes nothing else about
// it. Cancellation does not advance or touch the series hash chain, so no
// per-series lock is taken — a single transaction suffices.
//
// AT notification of the cancellation is a communication concern handled by a
// later plan; this method updates local state only.
func (s *InvoicingService) CancelDocument(
	ctx context.Context, tenantID string, number domain.DocNumber, reason string,
) (domain.SalesInvoice, error) {
	var cancelled domain.SalesInvoice
	err := s.uow.Run(ctx, tenantID, func(tx RepoSet) error {
		doc, gerr := tx.Documents().GetSalesInvoice(number)
		if gerr != nil {
			return newError(KindNotFound, fmt.Errorf("document %s: %w", number.Format(), gerr))
		}
		rectifiers, rerr := tx.Documents().LiveRectifyingNotes(number)
		if rerr != nil {
			return newError(KindInternal, fmt.Errorf("scan rectifying notes for %s: %w", number.Format(), rerr))
		}
		nums := make([]domain.DocNumber, len(rectifiers))
		for i, n := range rectifiers {
			nums[i] = n.Number
		}
		if verr := domain.ValidateNoLiveRectifier(nums); verr != nil {
			return newError(KindConflict, fmt.Errorf("cancel %s: %w", number.Format(), verr))
		}
		if cerr := doc.Cancel(reason, s.clock.Now()); cerr != nil {
			// already-cancelled / wrong-status / past-deadline are all state
			// conflicts; the wrapped cause stays reachable via errors.Is.
			return newError(KindConflict, fmt.Errorf("cancel %s: %w", number.Format(), cerr))
		}
		if serr := tx.Documents().SaveSalesInvoice(doc); serr != nil {
			return newError(KindInternal, fmt.Errorf("save cancellation: %w", serr))
		}
		cancelled = doc
		return nil
	})
	if err != nil {
		return domain.SalesInvoice{}, err
	}
	return cancelled, nil
}

func issueOptions(t Tenant) domain.IssueOptions {
	return domain.IssueOptions{
		Calendar:  t.Calendar,
		IssuerEAC: t.Company.EACCode,
		FSLimits:  t.FSLimits,
	}
}

func qrFor(t Tenant, sw config.SoftwareIdentity) domain.QRConfig {
	return domain.QRConfig{
		IssuerNIF:         t.Company.NIF,
		CertificateNumber: sw.CertificateNumber,
	}
}
