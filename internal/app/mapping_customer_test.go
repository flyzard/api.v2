package app

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func realNIF(t *testing.T) CustomerInput {
	return CustomerInput{TaxID: "502819472", Name: "Restaurante O Cantinho, Lda",
		Country: "PT", Address: &AddressInput{"Av. da Boavista, 1200", "Porto", "4100-130"}}
}

func TestRealNIFCustomerIDIsStableAcrossCalls(t *testing.T) {
	a, err1 := customerFrom(realNIF(t))
	b, err2 := customerFrom(realNIF(t))
	if err1 != nil || err2 != nil {
		t.Fatalf("customerFrom errored: %v %v", err1, err2)
	}
	if a.CustomerID != b.CustomerID {
		t.Fatal("same NIF must yield the same CustomerID, else NC/ND/RC allocation breaks")
	}
}

func TestTwoNamedFinalConsumersStayDistinctAndNonAnonymous(t *testing.T) {
	joao := CustomerInput{TaxID: string(domain.FinalConsumerNIF), Name: "João Pedro Martins",
		Country: "PT", Address: &AddressInput{"Rua do Sol, 12", "Caldas da Rainha", "2500-100"}}
	ana := CustomerInput{TaxID: string(domain.FinalConsumerNIF), Name: "Ana Rita Ferreira",
		Country: "PT", Address: &AddressInput{"Travessa da Igreja, 3", "Alcobaça", "2460-050"}}
	cj, ej := customerFrom(joao)
	ca, ea := customerFrom(ana)
	if ej != nil || ea != nil {
		t.Fatalf("named final consumers must validate: %v %v", ej, ea)
	}
	if cj.CustomerID == ca.CustomerID {
		t.Fatal("C004 and C005 collapsed to one identity")
	}
	if cj.IsAnonymous() || ca.IsAnonymous() {
		t.Fatal("named final consumers must NOT be anonymous (keeps FS ceiling + address validation)")
	}
}

func TestWalkInIsAnonymous(t *testing.T) {
	c, err := customerFrom(CustomerInput{TaxID: string(domain.FinalConsumerNIF), Name: "Consumidor final", Anonymous: true})
	if err != nil {
		t.Fatalf("walk-in: %v", err)
	}
	if !c.IsAnonymous() {
		t.Fatal("Anonymous=true must produce the reserved AnonymousCustomerID")
	}
}
