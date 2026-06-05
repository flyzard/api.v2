package domain

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// SourceDocState is the caller-fetched state of a document being consumed by
// an allocation — a receipt line settling an invoice (SourceDocumentID) or an
// NC/ND line rectifying one (References). The domain stays pure: persistence
// supplies the state, ValidateAllocations applies the rules.
type SourceDocState struct {
	CustomerID uuid.UUID      // customer the source document was issued to
	Status     DocumentStatus // current status; cancelled sources cannot be consumed
	Gross      Money          // source document GrossTotal
	Consumed   Money          // prior non-cancelled allocations against this document
}

// AllocationPolicy relaxes individual checks for the cases where the strict
// rule has no legal basis or blocks legitimate use.
type AllocationPolicy struct {
	// AllowUnknownSource skips validation for claims whose source document is
	// not in the system — e.g. an RG settling a pre-system invoice, or an NC
	// referencing a manually issued document.
	AllowUnknownSource bool
	// SkipCeiling disables the Consumed+claim <= Gross check. Needed for
	// rappel/volume-discount NCs that span many invoices, where per-source
	// allocation is not meaningful. The ceiling itself is stricter-safe, not
	// statutory ([CONFIRMAR] — no CIVA article caps NC amounts).
	SkipCeiling bool
}

// ValidateAllocations checks that the consuming document's claims — keyed by
// the source document number (OriginatingON / Reference) — are consistent
// with the source documents' state: source known, not cancelled, same
// customer, and cumulative allocations within the source GrossTotal.
func ValidateAllocations(customerID uuid.UUID, claims map[string]Money, sources map[string]SourceDocState, policy AllocationPolicy) error {
	keys := make([]string, 0, len(claims))
	for k := range claims {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic first-offender reporting

	for _, doc := range keys {
		claim := claims[doc]
		if claim <= 0 {
			return fmt.Errorf("allocation against %q must be positive, got %s", doc, claim.Format2DP())
		}
		src, ok := sources[doc]
		if !ok {
			if policy.AllowUnknownSource {
				continue
			}
			return fmt.Errorf("%w: %q", ErrUnknownSourceDoc, doc)
		}
		if src.Status == StatusCancelled {
			return fmt.Errorf("%w: %q", ErrSourceDocCancelled, doc)
		}
		if src.CustomerID != customerID {
			return fmt.Errorf("%w: %q", ErrSourceCustomerMismatch, doc)
		}
		if !policy.SkipCeiling && src.Consumed+claim > src.Gross {
			return fmt.Errorf("%w: %q consumed %s + claim %s > gross %s",
				ErrAllocationExceedsSource, doc,
				src.Consumed.Format2DP(), claim.Format2DP(), src.Gross.Format2DP())
		}
	}
	return nil
}
