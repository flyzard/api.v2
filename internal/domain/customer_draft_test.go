package domain

import (
	"strings"
	"testing"
)

func TestDraftValidate_RejectsInvalidCustomerNIF(t *testing.T) {
	d := gdDraft(t, nil)
	d.Customer.CustomerTaxID = "500000001" // fails PT checksum (check digit must be 0)
	err := d.Validate()
	if err == nil || !strings.Contains(err.Error(), "customer") {
		t.Fatalf("want customer validation error, got %v", err)
	}
}
