package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

var (
	allocCustomer = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherCustomer = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func sourceFT(gross, consumed Money) SourceDocState {
	return SourceDocState{
		CustomerID: allocCustomer,
		Status:     StatusNormal,
		Gross:      gross,
		Consumed:   consumed,
	}
}

func TestValidateAllocations(t *testing.T) {
	const ft = "FT S2026/1"

	t.Run("within_remaining_balance_ok", func(t *testing.T) {
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(40 * scale)},
			map[string]SourceDocState{ft: sourceFT(Money(100*scale), Money(50*scale))},
			AllocationPolicy{})
		if err != nil {
			t.Fatalf("valid allocation rejected: %v", err)
		}
	})

	t.Run("exact_remainder_ok", func(t *testing.T) {
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(50 * scale)},
			map[string]SourceDocState{ft: sourceFT(Money(100*scale), Money(50*scale))},
			AllocationPolicy{})
		if err != nil {
			t.Fatalf("exact-remainder allocation rejected: %v", err)
		}
	})

	t.Run("cumulative_overrun_rejected", func(t *testing.T) {
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(50*scale + 1)}, // remainder is exactly 50.00
			map[string]SourceDocState{ft: sourceFT(Money(100*scale), Money(50*scale))},
			AllocationPolicy{})
		if !errors.Is(err, ErrAllocationExceedsSource) {
			t.Fatalf("err = %v, want ErrAllocationExceedsSource", err)
		}
	})

	t.Run("unknown_source_rejected", func(t *testing.T) {
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(10 * scale)},
			map[string]SourceDocState{},
			AllocationPolicy{})
		if !errors.Is(err, ErrUnknownSourceDoc) {
			t.Fatalf("err = %v, want ErrUnknownSourceDoc", err)
		}
	})

	t.Run("unknown_source_allowed_by_policy", func(t *testing.T) {
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(10 * scale)},
			map[string]SourceDocState{},
			AllocationPolicy{AllowUnknownSource: true})
		if err != nil {
			t.Fatalf("policy-allowed unknown source rejected: %v", err)
		}
	})

	t.Run("cancelled_source_rejected", func(t *testing.T) {
		src := sourceFT(Money(100*scale), 0)
		src.Status = StatusCancelled
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(10 * scale)},
			map[string]SourceDocState{ft: src},
			AllocationPolicy{SkipCeiling: true}) // ceiling skip must not bypass status check
		if !errors.Is(err, ErrSourceDocCancelled) {
			t.Fatalf("err = %v, want ErrSourceDocCancelled", err)
		}
	})

	t.Run("customer_mismatch_rejected", func(t *testing.T) {
		err := ValidateAllocations(otherCustomer,
			map[string]Money{ft: Money(10 * scale)},
			map[string]SourceDocState{ft: sourceFT(Money(100*scale), 0)},
			AllocationPolicy{})
		if !errors.Is(err, ErrSourceCustomerMismatch) {
			t.Fatalf("err = %v, want ErrSourceCustomerMismatch", err)
		}
	})

	t.Run("skip_ceiling_allows_overrun", func(t *testing.T) {
		// Rappel/volume-discount NC: claims legitimately exceed per-source
		// remainder ([CONFIRMAR] ceiling rule itself).
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: Money(500 * scale)},
			map[string]SourceDocState{ft: sourceFT(Money(100*scale), Money(50*scale))},
			AllocationPolicy{SkipCeiling: true})
		if err != nil {
			t.Fatalf("skip-ceiling overrun rejected: %v", err)
		}
	})

	t.Run("non_positive_claim_rejected", func(t *testing.T) {
		err := ValidateAllocations(allocCustomer,
			map[string]Money{ft: 0},
			map[string]SourceDocState{ft: sourceFT(Money(100*scale), 0)},
			AllocationPolicy{})
		if err == nil {
			t.Fatal("zero claim accepted")
		}
	})
}
