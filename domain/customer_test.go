package domain

import (
	"encoding/json"
	"testing"
)

func validPTAddress(t *testing.T) Address {
	t.Helper()
	addr, err := NewAddress("Rua A 1", "Lisboa", "1000-001", "PT")
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func validESAddress(t *testing.T) Address {
	t.Helper()
	addr, err := NewAddress("Calle B 2", "Madrid", "28001", "ES")
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func TestCustomerPTRequiresValidNIF(t *testing.T) {
	addr := validPTAddress(t)
	// 111111111: prefix valid but checksum fails.
	if _, err := NewCustomer("ACC1", "111111111", "Acme Lda", addr, false); err == nil {
		t.Fatal("PT customer with invalid NIF: expected error")
	}
	// 503504564 is a known valid PT NIF (Microsoft Portugal).
	if _, err := NewCustomer("ACC1", "503504564", "Acme Lda", addr, false); err != nil {
		t.Fatalf("PT customer with valid NIF: %v", err)
	}
}

func TestCustomerForeignSkipsNIFChecksum(t *testing.T) {
	addr := validESAddress(t)
	// Spanish tax id wouldn't pass PT NIF; non-PT must still be accepted.
	if _, err := NewCustomer("ACC2", "X1234567A", "Iberia SA", addr, false); err != nil {
		t.Fatalf("ES customer: %v", err)
	}
}

func TestCustomerForeignRejectsOverlongTaxID(t *testing.T) {
	addr := validESAddress(t)
	long := "ABCDEFGHIJKLMNOPQRSTUVWXYZ12345" // 31 chars
	if _, err := NewCustomer("ACC2", CustomerTaxID(long), "X", addr, false); err == nil {
		t.Fatal("31-char tax id: expected error")
	}
}

func TestCustomerAccountIDDesconhecidoAccepted(t *testing.T) {
	addr := validPTAddress(t)
	if _, err := NewCustomer("Desconhecido", "503504564", "Acme", addr, false); err != nil {
		t.Fatalf("Desconhecido AccountID: %v", err)
	}
}

func TestCustomerAccountIDRejectsCaret(t *testing.T) {
	addr := validPTAddress(t)
	if _, err := NewCustomer("AC^1", "503504564", "Acme", addr, false); err == nil {
		t.Fatal("AccountID with '^': expected error")
	}
}

func TestCustomerUnmarshalRejectsBadPTNIF(t *testing.T) {
	// Shape-valid taxid (9 digits, prefix 1 OK) but checksum fails for PT.
	payload := []byte(`{
		"customer_id": "00000000-0000-0000-0000-000000000001",
		"account_id": "ACC1",
		"customer_tax_id": "111111111",
		"company_name": "Acme",
		"billing_address": {
			"address_detail": "Rua A 1",
			"city": "Lisboa",
			"postal_code": "1000-001",
			"country": "PT"
		}
	}`)
	var c Customer
	if err := json.Unmarshal(payload, &c); err == nil {
		t.Fatal("expected error unmarshalling PT customer with bad NIF")
	}
}

func TestCustomerUnmarshalAcceptsForeign(t *testing.T) {
	// Spanish tax id; non-PT country skips NIF checksum.
	payload := []byte(`{
		"customer_id": "00000000-0000-0000-0000-000000000001",
		"account_id": "ACC2",
		"customer_tax_id": "X1234567A",
		"company_name": "Iberia SA",
		"billing_address": {
			"address_detail": "Calle B 2",
			"city": "Madrid",
			"postal_code": "28001",
			"country": "ES"
		}
	}`)
	var c Customer
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestCustomerShipToAddressesIsSlice(t *testing.T) {
	addr := validPTAddress(t)
	c, err := NewCustomer("ACC1", "503504564", "Acme", addr, false)
	if err != nil {
		t.Fatal(err)
	}
	c.ShipToAddresses = append(c.ShipToAddresses, validESAddress(t), validPTAddress(t))
	if len(c.ShipToAddresses) != 2 {
		t.Fatalf("got %d addresses, want 2", len(c.ShipToAddresses))
	}
}
